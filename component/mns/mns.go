package mns

import (
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/go-spirit/spirit/component"
	"github.com/go-spirit/spirit/doc"
	"github.com/go-spirit/spirit/mail"
	"github.com/go-spirit/spirit/message"
	"github.com/go-spirit/spirit/protocol"
	"github.com/go-spirit/spirit/worker"
	"github.com/gogap/ali_mns"
)

type mnsQueue struct {
	Name            string
	Endpoint        string
	AccessKeyId     string
	AccessKeySecret string
	Queue           ali_mns.AliMNSQueue
}

type MNSComponent struct {
	opts component.Options

	queues []mnsQueue

	endpoint        string
	accessKeyId     string
	accessKeySecret string

	respChan chan ali_mns.BatchMessageReceiveResponse
	errChan  chan error

	stopC chan bool
}

func init() {
	component.RegisterComponent("mns", NewMNSComponent)
	doc.RegisterDocumenter("mns", &MNSComponent{})
}

func NewMNSComponent(opts ...component.Option) (comp component.Component, err error) {
	mnsComp := &MNSComponent{
		stopC:    make(chan bool),
		respChan: make(chan ali_mns.BatchMessageReceiveResponse, 30),
		errChan:  make(chan error, 30),
	}

	err = mnsComp.init(opts...)
	if err != nil {
		return
	}

	comp = mnsComp

	return
}

func (p *MNSComponent) init(opts ...component.Option) (err error) {

	for _, o := range opts {
		o(&p.opts)
	}

	if p.opts.Config == nil {
		err = errors.New("mns component config is nil")
		return
	}

	akId := p.opts.Config.GetString("access-key-id")
	akSecret := p.opts.Config.GetString("access-key-secret")
	endpoint := p.opts.Config.GetString("endpoint")

	p.accessKeyId = akId
	p.accessKeySecret = akSecret
	p.endpoint = endpoint

	queuesConf := p.opts.Config.GetConfig("queues")

	if queuesConf == nil {
		return
	}

	qNames := queuesConf.Keys()

	var mnsQueues []mnsQueue

	for _, name := range qNames {
		endpoint := queuesConf.GetString("endpoint", endpoint)
		qAkId := queuesConf.GetString("access-key-id", akId)
		qAkSecret := queuesConf.GetString("access-key-secret", akSecret)

		qClient := ali_mns.NewAliMNSClient(endpoint, qAkId, qAkSecret)
		aliQueue := ali_mns.NewMNSQueue(name, qClient)

		q := mnsQueue{
			Name:            name,
			Endpoint:        endpoint,
			AccessKeyId:     qAkId,
			AccessKeySecret: qAkSecret,
			Queue:           aliQueue,
		}

		mnsQueues = append(mnsQueues, q)
	}

	p.queues = mnsQueues

	return
}

func (p *MNSComponent) Start() error {
	for _, q := range p.queues {
		mgr := ali_mns.NewMNSQueueManager(q.AccessKeyId, q.AccessKeySecret)
		_, err := mgr.GetQueueAttributes(q.Endpoint, q.Name)
		if err != nil {
			return err
		}
	}

	for _, q := range p.queues {
		go q.Queue.BatchReceiveMessage(p.respChan, p.errChan, 16, 30)
	}

	go p.receiveMessage()

	return nil
}

func (p *MNSComponent) postMessage(resp ali_mns.MessageReceiveResponse) {

	payload := &protocol.Payload{}
	err := protocol.Unmarshal(resp.MessageBody, payload)

	if err != nil {
		return
	}

	port, err := payload.GetGraph().CurrentPort()

	if err != nil {
		return
	}

	session := mail.NewSession()

	session.WithPayload(payload)
	session.WithFromTo("", port.GetUrl())

	err = p.opts.Postman.Post(
		message.NewUserMessage(session),
	)

	if err != nil {
		return
	}
}

func (p *MNSComponent) receiveMessage() {
	for {
		select {
		case resp, ok := <-p.respChan:
			{
				if !ok {
					break
				}

				for _, m := range resp.Messages {
					p.postMessage(m)
				}
			}
		case err, ok := <-p.errChan:
			{
				if !ok {
					break
				}

				fmt.Println(err)
			}
		case <-p.stopC:
			{
				if len(p.respChan) > 0 || len(p.errChan) > 0 {
					continue
				}
				p.stopC <- true
				break
			}
		}
	}
}

func (p *MNSComponent) Stop() error {

	if len(p.queues) == 0 {
		return nil
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(p.queues))

	for _, q := range p.queues {
		go func() {
			defer wg.Done()
			q.Queue.Stop()
		}()
	}

	wg.Wait()

	p.stopC <- true
	<-p.stopC

	close(p.errChan)
	close(p.respChan)
	close(p.stopC)

	return nil
}

func (p *MNSComponent) sendMessage(session mail.Session) (err error) {

	port := session.Value(worker.CtxKeyPort{}).(*worker.CtxValuePort)

	toURL, err := url.Parse(port.To)
	if err != nil {
		err = fmt.Errorf("parse port to url failure")
		return
	}

	queueName := toURL.Query().Get("queue")

	if len(queueName) == 0 {
		err = fmt.Errorf("queue name is empty")
		return
	}

	endpoint, exist := port.Metadata["endpoint"]

	if !exist {
		endpoint = p.endpoint
	}

	akId, exist := port.Metadata["access-key-id"]
	if !exist {
		akId = p.accessKeyId
	}

	akSecret, exist := port.Metadata["access-key-secret"]

	if !exist {
		akSecret = p.accessKeySecret
	}

	if len(endpoint) == 0 || len(akId) == 0 || len(akSecret) == 0 {
		err = fmt.Errorf("error mns send params in msn component, port to url: %s", port.To)
		return
	}

	client := ali_mns.NewAliMNSClient(endpoint, akId, akSecret)
	queue := ali_mns.NewMNSQueue(queueName, client)

	payload := session.Payload().(*protocol.Payload)

	data, err := payload.ToBytes()

	if err != nil {
		return
	}

	req := ali_mns.MessageSendRequest{
		MessageBody: data,
		Priority:    8,
	}

	_, err = queue.SendMessage(req)

	if err != nil {
		return
	}

	return
}

func (p *MNSComponent) Handler() worker.HandlerFunc {
	return p.sendMessage
}

func (p *MNSComponent) Document() doc.Document {

	document := doc.Document{
		Title:       "MNS Sender And Receiver",
		Description: "MNS is aliyun message service, we could receive queue message from msn and send message to msn queue",
	}

	return document
}

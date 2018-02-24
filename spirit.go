package spirit

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/go-spirit/spirit/cache"
	"github.com/go-spirit/spirit/component"
	"github.com/go-spirit/spirit/doc"
	"github.com/go-spirit/spirit/mail"
	"github.com/go-spirit/spirit/worker"
	"github.com/gogap/config"

	_ "github.com/go-spirit/spirit/cache/gocache"
	_ "github.com/go-spirit/spirit/mail/mailbox"
	_ "github.com/go-spirit/spirit/mail/postman/tiny"
	_ "github.com/go-spirit/spirit/mail/registry/tiny"
)

var (
	ErrNameIsEmpty = errors.New("name param is empty")
)

type Spirit struct {
	loc sync.Mutex

	postman mail.Postman
	reg     mail.Registry
	cache   cache.Cache

	workers map[string]worker.Worker
	actors  map[string]*Actor

	conf config.Configuration
}

func New(opts ...Option) (s *Spirit, err error) {

	spiritOpts := &Options{
		config: config.NewConfig(),
	}

	for _, o := range opts {
		o(spiritOpts)
	}

	reg, err := mail.NewRegistry("tiny")
	if err != nil {
		return
	}

	man, err := mail.NewPostman("tiny", mail.PostmanRegistry(reg))
	if err != nil {
		return
	}

	defaultCache, err := cache.NewCache("go-cache",
		cache.Config(
			spiritOpts.config.GetConfig("cache"),
		),
	)

	sp := &Spirit{
		reg:     reg,
		postman: man,
		cache:   defaultCache,
		workers: make(map[string]worker.Worker),
		actors:  make(map[string]*Actor),
		conf:    spiritOpts.config,
	}

	err = sp.generateActors()
	if err != nil {
		return
	}

	s = sp

	return
}

func (p *Spirit) generateActors() (err error) {
	componentsConf := p.conf.GetConfig("components")
	if componentsConf == nil {
		return
	}

	drivers := componentsConf.Keys()

	if len(drivers) == 0 {
		return
	}

	for _, driver := range drivers {

		driverConf := componentsConf.GetConfig(driver)

		if driverConf == nil {
			continue
		}

		actorNames := driverConf.Keys()

		if len(actorNames) == 0 {
			continue
		}

		for _, actName := range actorNames {
			_, err = p.NewActor(
				actName,
				ActorComponent(driver, component.Config(
					driverConf.GetConfig(actName),
				)),
			)

			if err != nil {
				return
			}
		}
	}

	return
}

func (p *Spirit) Start() (err error) {

	for _, act := range p.actors {
		err = act.Start()
		if err != nil {
			return
		}
	}

	err = p.postman.Start()
	if err != nil {
		return
	}

	return
}

func (p *Spirit) Stop() (err error) {
	for _, act := range p.actors {
		act.Stop()
	}

	return nil
}

func (p *Spirit) WithPostman(pm mail.Postman) {
	p.postman = pm
}

func (p *Spirit) newWorker(name string, opts ...WorkerOption) (wk worker.Worker, err error) {

	if len(name) == 0 {
		err = ErrNameIsEmpty
		return
	}

	key := fmt.Sprintf("workers.%s", name)

	driver := p.conf.GetString(key+".driver", "fbp")
	if len(driver) == 0 {
		err = fmt.Errorf("the driver of worker is empty", name)
		return
	}

	workerOptions := WorkerOptions{
		Url: fmt.Sprintf("spirit://workers/%s", name),
	}

	for _, o := range opts {
		o(&workerOptions)
	}

	newWk, err := worker.New(
		driver,
		worker.Postman(p.postman),
		worker.Handler(workerOptions.Handler),
	)

	if err != nil {
		return
	}

	mailboxDriver := p.conf.GetString(key+"mail.mailbox.driver", "unbounded")

	box, err := mail.NewMailbox(
		mailboxDriver,
		mail.MailboxUrl(workerOptions.Url),
		mail.MailboxMessageInvoker(newWk),
	)

	if err != nil {
		return
	}

	err = p.reg.Register(box)
	if err != nil {
		return
	}

	wk = newWk

	return
}

func (p *Spirit) NewActor(name string, opts ...ActorOption) (act *Actor, err error) {

	actOpts := ActorOptions{}

	for _, o := range opts {
		o(&actOpts)
	}

	if len(actOpts.componentDriver) == 0 {
		err = errors.New("component driver name is empty")
		return
	}

	if len(actOpts.url) == 0 {
		actOpts.url = fmt.Sprintf("spirit://actors/%s/%s", actOpts.componentDriver, name)
	}

	componentConf := p.conf.GetConfig(fmt.Sprintf("components.%s.%s", actOpts.componentDriver, name))
	if componentConf == nil {
		componentConf = config.NewConfig()
	}

	compOptions := []component.Option{
		component.Postman(p.postman),
		component.Cache(p.cache),
		component.Config(componentConf),
	}

	compOptions = append(compOptions, actOpts.componentOptions...)

	comp, err := component.NewComponent(
		actOpts.componentDriver,
		compOptions...,
	)

	if err != nil {
		return
	}

	warnNoDocsComp(name, actOpts.componentDriver, comp)

	worker, err := p.newWorker(
		name,
		WorkerUrl(actOpts.url),
		WorkerHandler(comp.Handler()),
	)

	if err != nil {
		return
	}

	act = &Actor{
		worker:    worker,
		component: comp,
	}

	p.actors[name] = act

	return
}

func warnNoDocsComp(name, driver string, comp interface{}) {
	if _, ok := comp.(doc.Documenter); !ok {
		if driver == "function" {
			if _, exist := doc.GetDocumenter(name); !exist {
				logrus.WithField("function", name).Warnln("no document implement")
			}
		} else {
			logrus.WithField("component-driver", driver).Warnln("no document implement")
		}
	} else if _, exist := doc.GetDocumenter(driver); !exist {
		logrus.WithField("component-driver", driver).Warnln("document implemented, but not registered")
	}
}
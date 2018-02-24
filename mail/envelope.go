package mail

import (
	"fmt"
	"time"

	"github.com/go-spirit/spirit/context"
)

type ctxKeyPayload struct{}
type ctxKeyFrom struct{}
type ctxKeyTo struct{}

type Message interface {
	message()
}

type SystemMessage interface {
	Message
	systemMessage()
}

type UserMessage interface {
	Message
	userMessage()
	Session() Session
}

type Contenter interface {
	Content() Content
}

type Session interface {
	From() string
	To() string
	WithFromTo(from, to string)

	Payload() interface{}
	WithPayload(interface{})

	Err() error
	WithError(error)

	WithValue(key interface{}, value interface{})
	Value(key interface{}) interface{}

	Done() <-chan struct{}
	Deadline() (deadline time.Time, ok bool)

	Fork() Session

	PayloadContent() Content

	String() string
}

type defaultSession struct {
	ctx context.Context
}

type SessionOption func(*SessionOptions)

type SessionOptions struct {
	context context.Context
}

func SessionContext(context context.Context) SessionOption {
	return func(s *SessionOptions) {
		s.context = context
	}
}

func NewSession(opts ...SessionOption) Session {

	sOpts := SessionOptions{}

	for _, o := range opts {
		o(&sOpts)
	}

	if sOpts.context == nil {
		sOpts.context = context.NewContext()
	}

	return &defaultSession{
		ctx: sOpts.context,
	}
}

func (p *defaultSession) From() string {
	from, ok := p.ctx.Value(ctxKeyFrom{}).(string)
	if ok {
		return from
	}

	return ""
}

func (p *defaultSession) To() string {
	to, ok := p.ctx.Value(ctxKeyTo{}).(string)
	if ok {
		return to
	}

	return ""
}

func (p *defaultSession) Err() error {
	return p.ctx.Err()
}

func (p *defaultSession) Value(key interface{}) interface{} {
	return p.ctx.Value(key)
}

func (p *defaultSession) Payload() interface{} {
	return p.ctx.Value(ctxKeyPayload{})
}

func (p *defaultSession) WithPayload(payload interface{}) {
	p.ctx.WithValue(ctxKeyPayload{}, payload)
}

func (p *defaultSession) Fork() Session {
	return &defaultSession{
		ctx: p.ctx.Copy(),
	}
}

func (p *defaultSession) WithFromTo(from, to string) {
	p.ctx.WithValue(ctxKeyFrom{}, from)
	p.ctx.WithValue(ctxKeyTo{}, to)
}

func (p *defaultSession) WithError(err error) {
	p.ctx.WithError(err)
}

func (p *defaultSession) WithValue(key, value interface{}) {
	p.ctx.WithValue(key, value)
}

func (p *defaultSession) Done() <-chan struct{} {
	return p.ctx.Done()
}

func (p *defaultSession) Deadline() (deadline time.Time, ok bool) {
	return p.ctx.Deadline()
}

func (p *defaultSession) PayloadContent() Content {
	pay, ok := p.Payload().(Contenter)
	if !ok {
		return nil
	}

	return pay.Content()
}

func (p *defaultSession) String() string {
	from := p.From()
	to := p.To()
	err := p.Err()

	return fmt.Sprintf("From: %s, To: %s, Error: %v", from, to, err)
}

type Content interface {
	GetId() string
	GetHeader() map[string]string
	GetBody() []byte
	SetBody(interface{}) error
	Copy() Content
	String() string

	SetError(err error)

	ContentType() (ct string, exist bool)
	Unmarshal(v interface{}) (err error)
}
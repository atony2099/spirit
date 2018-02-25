package spirit

import (
	"github.com/gogap/config"
	// "github.com/go-spirit/spirit/mail"
	"github.com/go-spirit/spirit/worker"
)

type WorkerOptions struct {
	Url     string
	Handler worker.HandlerFunc
}

type WorkerOption func(*WorkerOptions)

func WorkerUrl(url string) WorkerOption {
	return func(w *WorkerOptions) {
		w.Url = url
	}
}

func WorkerHandler(h worker.HandlerFunc) WorkerOption {
	return func(w *WorkerOptions) {
		w.Handler = h
	}
}

type Options struct {
	config config.Configuration
}

type Option func(*Options)

func ConfigFile(filename string) Option {
	return func(o *Options) {

		o.config.WithFallback(
			config.NewConfig(
				config.ConfigFile(filename),
			),
		)
	}
}

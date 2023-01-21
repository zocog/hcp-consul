package scada

import (
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	libscada "github.com/hashicorp/hcp-scada-provider"
)

var _ Provider = &watchdogProvider{}

type watchdogProvider struct {
	Provider
	mu         sync.Mutex
	running    bool
	shutdownCh chan struct{}
	interval   time.Duration
	logger     hclog.Logger
}

func (p *watchdogProvider) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return nil
	}

	if err := p.Provider.Start(); err != nil {
		return err
	}

	p.running = true
	p.shutdownCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-p.shutdownCh:
				return
			case <-time.After(p.interval):
				if p.Provider.SessionStatus() == libscada.SessionStatusDisconnected {
					p.logger.Debug("scada provider reconnecting")
					if err := p.Provider.Start(); err != nil {
						p.logger.Warn("scada provider failed to reconnect", "error", err, "next_retry_after", p.interval.String())
					}
				}
			}
		}
	}()

	return nil
}

func (p *watchdogProvider) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return libscada.ErrProviderNotStarted
	}

	err := p.Provider.Stop()
	if err != nil && err != libscada.ErrProviderNotStarted {
		return err
	}

	p.running = false
	close(p.shutdownCh)
	return err
}

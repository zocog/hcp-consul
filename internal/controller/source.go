package controller

import "context"

// Watch is responsible for watching for custom events from source and adding them to
// the event queue.
func (s *Source) Watch(ctx context.Context, add func(e Event)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-s.Source:
			add(evt)
		}
	}
}

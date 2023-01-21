package scada

import (
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/testutil"
	libscada "github.com/hashicorp/hcp-scada-provider"
	"github.com/stretchr/testify/require"
)

func Test_watchdogProvider(t *testing.T) {
	testInterval := 10 * time.Millisecond
	for name, _tc := range map[string]struct {
		states             []libscada.SessionStatus
		stopAfterStateIdx  int
		expectedStartCalls int
	}{
		"Nominal": {
			states: []libscada.SessionStatus{
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
			},
			stopAfterStateIdx:  -1,
			expectedStartCalls: 1,
		},
		"EarlyStop": {
			states: []libscada.SessionStatus{
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
			},
			expectedStartCalls: 1,
		},
		"SingleDisconnection": {
			states: []libscada.SessionStatus{
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
			},
			stopAfterStateIdx:  -1,
			expectedStartCalls: 2,
		},
		"MultipleDisconnections": {
			states: []libscada.SessionStatus{
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
			},
			stopAfterStateIdx:  -1,
			expectedStartCalls: 4,
		},
		"MultipleDisconnectionsEarlyStop": {
			states: []libscada.SessionStatus{
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
				libscada.SessionStatusConnecting,
				libscada.SessionStatusConnected,
				libscada.SessionStatusDisconnected,
			},
			stopAfterStateIdx:  4,
			expectedStartCalls: 2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc := _tc
			provider := NewMockProvider(t)
			wd := &watchdogProvider{
				Provider: provider,
				interval: testInterval,
				logger:   testutil.Logger(t),
			}

			statusIdx := 0
			provider.On("SessionStatus").Return(func() libscada.SessionStatus {
				status := tc.states[0]
				if statusIdx == tc.stopAfterStateIdx || len(tc.states) == 1 {
					defer require.NoError(t, wd.Stop())
				}
				if len(tc.states) > 1 {
					tc.states = tc.states[1:]
					statusIdx += 1
				}
				return status
			})
			provider.EXPECT().Start().Return(nil).Times(tc.expectedStartCalls)
			provider.EXPECT().Stop().Return(nil).Once()

			require.False(t, wd.running)
			require.NoError(t, wd.Start())
			require.True(t, wd.running)
			ms := time.Duration(len(tc.states)+1) * testInterval
			<-time.After(ms)
			require.False(t, wd.running)
			require.ErrorIs(t, wd.Stop(), libscada.ErrProviderNotStarted)
		})
	}
}

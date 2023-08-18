// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package apply

import (
	"errors"
	"testing"

	"github.com/hashicorp/consul/agent"
	"github.com/hashicorp/consul/testrpc"
	"github.com/mitchellh/cli"
	"github.com/stretchr/testify/require"
)

func TestResourceApplyCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("too slow for testing.Short")
	}

	t.Parallel()
	a := agent.NewTestAgent(t, ``)
	defer a.Shutdown()
	testrpc.WaitForTestAgent(t, a.RPC, "dc1")

	cases := []struct {
		name      string
		output    string
	}{
		{
			name:   "sample output",
			output: "demo.v2.Artist 'korn' created.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			c := New(ui)
			args := []string{
				"-f=../testdata/demo.hcl",
				"-http-addr=" + a.HTTPAddr(),
				"-token=root",
			}

			code := c.Run(args)
			require.Equal(t, 0, code)
			require.Empty(t, ui.ErrorWriter.String())
			require.Contains(t, ui.OutputWriter.String(), tc.output)
		})
	}
}

func TestResourceApplyInvalidArgs(t *testing.T) {
	t.Parallel()

	type tc struct {
		args           []string
		expectedCode   int
		expectedErr    error
	}

	cases := map[string]tc{
		"missing file path": {
			args:           []string{"-f"},
			expectedCode:   1,
			expectedErr: 	errors.New("Failed to parse args: flag needs an argument: -f"),
		},
	}

	for desc, tc := range cases {
		t.Run(desc, func(t *testing.T) {
			ui := cli.NewMockUi()
			c := New(ui)

			code := c.Run(tc.args)

			require.Equal(t, tc.expectedCode, code)
			require.Contains(t, ui.ErrorWriter.String(), tc.expectedErr.Error())
		})
	}
}

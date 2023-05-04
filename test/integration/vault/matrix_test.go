package test

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

// tests to run in matrix
type testFunc func(*testing.T, TestConsulServer, TestVaultServer)

var matrixTests = []testFunc{
	tutorial,
	testConsulManagedACLs,
	testVaultManagedACLs,
}

// The matrix test
func TestMatrix(t *testing.T) {
	matrix := NewMatrix()
	for i := 0; i < len(matrix.pairs); i++ {
		consul, vault, more := matrix.NextPair(t)
		if !more {
			return
		}
		for _, test := range matrixTests {
			funName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
			testName := fmt.Sprintf("%s-consul_%v-vault_%v",
				funName, consul.version, vault.version)
			t.Run(testName, func(t *testing.T) {
				test(t, consul, vault)
			})
			consul, vault = maybeRestart(t, consul, vault)
		}
		consul.Stop()
		vault.Stop()
	}
}

func maybeRestart(t *testing.T, c TestConsulServer, v TestVaultServer) (TestConsulServer, TestVaultServer) {
	if c.needsRestart() {
		t.Log("Restarting Consul")
		c.Stop()
		c = NewTestConsulServer(t, "consul", "local")
	}

	if v.needsRestart() {
		t.Log("Restarting Vault")
		v.Stop()
		v = NewTestVaultServer(t, "vault", "local")
	}
	return c, v
}

// organizes matrix tests between 2 products
type Matrix struct {
	consulVersions, vaultVersions []string
	pairs                         []pair
}

// Returns a matrix ready for use in testing
func NewMatrix() Matrix {
	cvs := latestReleases("consul", 1)
	vvs := latestReleases("vault", 3)
	pairs := make([]pair, 0, (3 * 1))
	for _, cv := range cvs {
		for _, vv := range vvs {
			pairs = append(pairs, pair{vault: vv, consul: cv})
		}
	}
	return Matrix{
		consulVersions: cvs,
		vaultVersions:  vvs,
		pairs:          pairs,
	}
}

// iterates through the matrix binary pairs
func (m Matrix) NextPair(t *testing.T) (TestConsulServer, TestVaultServer, bool) {
	nextPair := m.next()
	if nextPair.Nil() {
		return TestConsulServer{}, TestVaultServer{}, false
	}
	return NewTestConsulServer(t, getBinary("consul", nextPair.consul), nextPair.consul),
		NewTestVaultServer(t, getBinary("vault", nextPair.vault), nextPair.vault), true
}

type pair struct {
	vault, consul string
}

func (p pair) Nil() bool {
	return p.consul == "" || p.vault == ""
}

func (m Matrix) next() pair {
	for i, p := range m.pairs {
		if !p.Nil() {
			m.pairs[i] = pair{}
			return p
		}
	}
	return pair{}
}

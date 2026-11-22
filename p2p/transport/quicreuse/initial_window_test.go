package quicreuse

import (
	"context"
	"net"
	"testing"
	"testing/synctest"

	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/quic-go/quic-go"
	"github.com/stretchr/testify/require"
)

func TestInitialReceiveWindows(t *testing.T) {
	cm, err := NewConnManager(quic.StatelessResetKey{}, quic.TokenGeneratorKey{}, InitialReceiveWindows(128<<10, 1<<20))
	require.NoError(t, err)
	defer cm.Close()
	for _, c := range []*quic.Config{cm.clientConfig, cm.serverConfig} {
		require.Equal(t, uint64(128<<10), c.InitialStreamReceiveWindow)
		require.Equal(t, uint64(1<<20), c.InitialConnectionReceiveWindow)
	}
	require.Zero(t, cm.InitialConnectionWindowReservation(), "no reservation without AccountInitialConnectionWindow")

	for _, w := range [][2]uint64{{0, 1 << 20}, {128 << 10, 0}, {2 << 20, 1 << 20}, {11 << 20, 16 << 20}} {
		_, err := NewConnManager(quic.StatelessResetKey{}, quic.TokenGeneratorKey{}, InitialReceiveWindows(w[0], w[1]))
		require.Error(t, err, "windows %v", w)
	}
}

func TestAccountInitialConnectionWindow(t *testing.T) {
	rm := &network.NullResourceManager{}
	cm, err := NewConnManager(quic.StatelessResetKey{}, quic.TokenGeneratorKey{}, InitialReceiveWindows(128<<10, 1<<20), AccountInitialConnectionWindow(rm))
	require.NoError(t, err)
	defer cm.Close()
	require.Equal(t, 1<<20, cm.InitialConnectionWindowReservation())

	def, err := NewConnManager(quic.StatelessResetKey{}, quic.TokenGeneratorKey{}, AccountInitialConnectionWindow(rm))
	require.NoError(t, err)
	defer def.Close()
	require.Equal(t, defaultInitialConnectionReceiveWindow, def.InitialConnectionWindowReservation(), "quic-go's default window")
}

type countedScope struct {
	network.NullScope
	reserved int
	closed   int
}

func (s *countedScope) ReserveMemory(n int, _ uint8) error {
	s.reserved += n
	return nil
}

func (s *countedScope) Done() { s.closed++ }

type countedManager struct {
	network.NullResourceManager
	scope countedScope
}

func (m *countedManager) OpenConnection(network.Direction, bool, ma.Multiaddr) (network.ConnManagementScope, error) {
	return &m.scope, nil
}

func TestInboundWindowScope(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const window = 1 << 20
		rm := &countedManager{}
		cm, err := NewConnManager(quic.StatelessResetKey{}, quic.TokenGeneratorKey{}, InitialReceiveWindows(128<<10, window), AccountInitialConnectionWindow(rm))
		require.NoError(t, err)
		defer cm.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ctx, err = cm.connContext(ctx, &quic.ClientInfo{RemoteAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 42}})
		require.NoError(t, err)
		require.Equal(t, window, rm.scope.reserved)
		scope, err := network.UnwrapConnManagementScope(ctx)
		require.NoError(t, err)
		cancel()
		synctest.Wait()
		require.Equal(t, 1, rm.scope.closed, "a connection that never reaches Accept releases its scope")
		scope.Done()
		require.Equal(t, 1, rm.scope.closed, "a competing libp2p close cannot release the scope twice")
	})
}

package quicreuse

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/quic-go/quic-go"
)

type Option func(*ConnManager) error

type listenUDP func(network string, laddr *net.UDPAddr) (net.PacketConn, error)

func OverrideListenUDP(f listenUDP) Option {
	return func(m *ConnManager) error {
		m.listenUDP = f
		return nil
	}
}

func OverrideSourceIPSelector(f func() (SourceIPSelector, error)) Option {
	return func(m *ConnManager) error {
		m.sourceIPSelectorFn = f
		return nil
	}
}

func WithQlogTracerDir(dir string) Option {
	return func(m *ConnManager) error {
		m.qlogTracerDir = dir
		return nil
	}
}

func DisableReuseport() Option {
	return func(m *ConnManager) error {
		m.enableReuseport = false
		return nil
	}
}

// ConnContext sets the context for all connections accepted by listeners. This doesn't affect the
// context for dialed connections. To reject a connection, return a non nil error.
func ConnContext(f func(ctx context.Context, clientInfo *quic.ClientInfo) (context.Context, error)) Option {
	return func(m *ConnManager) error {
		if m.connContext != nil {
			return errors.New("cannot set ConnContext more than once")
		}
		m.connContext = f
		return nil
	}
}

// VerifySourceAddress returns whether to verify the source address for incoming connection requests.
// For more details see: `quic.Transport.VerifySourceAddress`
func VerifySourceAddress(f func(addr net.Addr) bool) Option {
	return func(m *ConnManager) error {
		m.verifySourceAddress = f
		return nil
	}
}

// EnableMetrics enables Prometheus metrics collection. If reg is nil,
// prometheus.DefaultRegisterer will be used as the registerer.
func EnableMetrics(reg prometheus.Registerer) Option {
	return func(m *ConnManager) error {
		m.enableMetrics = true
		if reg != nil {
			m.registerer = reg
		}
		return nil
	}
}

// InitialReceiveWindows sets the initial stream and connection receive windows of every QUIC connection
// (quic.Config's InitialStreamReceiveWindow and InitialConnectionReceiveWindow). quic-go's auto-tuning still grows
// them, up to the maximum windows, as the application reads. stream must be at most conn, and both at most the
// maximum windows (10 MB per stream, 15 MB per connection).
//
// Bytes a peer sent and the application did not read hold connection credit: with streams that are deliberately
// left unread (an application that orders its inbound streams, say), a connection window larger than the sum of
// their stream windows keeps credit for the stream being read.
func InitialReceiveWindows(stream, conn uint64) Option {
	return func(m *ConnManager) error {
		if stream == 0 || conn == 0 || stream > conn || stream > quicConfig.MaxStreamReceiveWindow || conn > quicConfig.MaxConnectionReceiveWindow {
			return fmt.Errorf("quicreuse: invalid initial receive windows (stream %d, connection %d)", stream, conn)
		}
		m.initialStreamWindow, m.initialConnWindow = stream, conn
		return nil
	}
}

// AccountInitialConnectionWindow reserves the initial receive window in the resource scope of each QUIC
// connection. It installs the inbound ConnContext callback and cannot be combined with ConnContext. Pass
// the same resource manager as the QUIC transport. Connections are refused if the window cannot be reserved.
func AccountInitialConnectionWindow(rm network.ResourceManager) Option {
	return func(m *ConnManager) error {
		if err := ConnContext(m.inboundScope(rm))(m); err != nil {
			return err
		}
		m.accountInitialWindow = true
		return nil
	}
}

package reconnect

type connection interface {
	// Connect will stablish a connection
	Connect() error

	// Wait will block until the connection drops
	Wait() error

	// Close closes down the connection
	Close() error
}

type reconnect struct {
	conn    connection
	opts    Options
	closing bool
}

func (c *reconnect) Start() error {
	var (
		connectAttempts  int
		connectionErrors int
		opts             = c.opts
	)
	notifyState(opts.NotifyState, StateConnecting)
	for !c.closing {
		var err error
		if err = c.conn.Connect(); err != nil {
			notifyError(opts.NotifyError, err)
			notifyState(opts.NotifyState, StateFailing)
			connectAttempts++
		} else {
			notifyState(opts.NotifyState, StateConnected)
			connectAttempts = 0
		}
		if opts.MaxConnectAttempts > 0 && connectAttempts == opts.MaxConnectAttempts {
			notifyState(opts.NotifyState, StateFailed)
			return err
		}
		if err != nil {
			notifyState(opts.NotifyState, StateReconnecting)
			continue
		}
		if err = c.conn.Wait(); err != nil {
			notifyError(opts.NotifyError, err)
			notifyState(opts.NotifyState, StateFailing)
			connectionErrors++
		} else {
			notifyState(opts.NotifyState, StateDisconnected)
			connectionErrors = 0
		}
		if opts.MaxConnectionErrors > 0 && connectionErrors == opts.MaxConnectionErrors {
			notifyState(opts.NotifyState, StateFailed)
			return err
		}
		notifyState(opts.NotifyState, StateReconnecting)
	}
	notifyState(opts.NotifyState, StateClosed)
	return nil
}

func notifyError(fn func(error), err error) {
	if fn != nil {
		fn(err)
	}
}

func notifyState(fn func(ConnState), state ConnState) {
	if fn != nil {
		fn(state)
	}
}

func (c *reconnect) Close() error {
	c.closing = true
	return c.conn.Close()
}

type closer interface {
	Close() error
}

// ConnState represents the state of the connection
type ConnState int

const (
	// StateConnecting is sent on first connection
	StateConnecting ConnState = iota
	// StateReconnecting is sent when reconnecting after a connection error
	StateReconnecting
	// StateConnected is sent after connecting
	StateConnected
	// StateClosed is sent once the connection is closed
	StateClosed
	// StateFailing is sent when failing connecting or connected
	StateFailing
	// StateFailed is sent when the connection fails
	StateFailed
	// StateDisconnected is sent when connection is disconnected with no errors
	StateDisconnected
)

func (s ConnState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateReconnecting:
		return "reconnecting"
	case StateConnected:
		return "connected"
	case StateClosed:
		return "closed"
	case StateFailed:
		return "failed"
	case StateFailing:
		return "failing"
	case StateDisconnected:
		return "disconnected"
	default:
		panic("unknown state")
	}
}

// Options to manage reconnection
type Options struct {
	MaxConnectAttempts  int
	MaxConnectionErrors int
	NotifyError         func(error)
	NotifyState         func(ConnState)
}

// New init
func New(c connection, params ...func(*Options)) *reconnect {
	opts := Options{}
	for _, fn := range params {
		fn(&opts)
	}
	r := reconnect{conn: c, opts: opts}
	return &r
}

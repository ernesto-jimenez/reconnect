package reconnect

type connection interface {
	// Connect will stablish a connection
	Connect() error

	// Wait will block until the connection drops
	Wait() error

	// Close closes down the connection
	Close() error
}

// Reconnect is returned by New
type Reconnect interface {
	// Start inits the reconnect process and blocks until closed or failed
	Start() error
	// Close closes the underlying connection
	Close() error
}

type reconnect struct {
	conn    connection
	opts    Options
	closing chan struct{}
	closed  chan struct{}
}

func (c *reconnect) Start() error {
	var (
		connectAttempts  int
		connectionErrors int
		opts             = c.opts
		stopErr          error
	)
	defer func() {
		close(c.closed)
	}()
	notifyState(opts.NotifyState, StateConnecting)
	for {
		select {
		case <-c.closing:
			notifyState(opts.NotifyState, StateClosed)
			return nil
		default:
		}
		var err error
		if err = c.conn.Connect(); err != nil {
			stopErr = notifyError(opts.NotifyError, err)
			notifyState(opts.NotifyState, StateFailing)
			connectAttempts++
		} else {
			notifyState(opts.NotifyState, StateConnected)
			connectAttempts = 0
		}
		if stopErr != nil {
			notifyState(opts.NotifyState, StateFailed)
			return stopErr
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
			stopErr = notifyError(opts.NotifyError, err)
			notifyState(opts.NotifyState, StateFailing)
			connectionErrors++
		} else {
			notifyState(opts.NotifyState, StateDisconnected)
			connectionErrors = 0
		}
		if stopErr != nil {
			notifyState(opts.NotifyState, StateFailed)
			return stopErr
		}
		if opts.MaxConnectionErrors > 0 && connectionErrors == opts.MaxConnectionErrors {
			notifyState(opts.NotifyState, StateFailed)
			return err
		}
		select {
		case <-c.closing:
		default:
			notifyState(opts.NotifyState, StateReconnecting)
		}
	}
}

func notifyError(fn func(error) error, err error) error {
	if fn != nil {
		return fn(err)
	}
	return nil
}

func notifyState(fn func(ConnState), state ConnState) {
	if fn != nil {
		fn(state)
	}
}

func (c *reconnect) Close() error {
	close(c.closing)
	err := c.conn.Close()
	<-c.closed
	return err
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
	// Max amount consecutive calls to Connect() returning an error
	MaxConnectAttempts int
	// Max amount of errors returned by Wait()
	MaxConnectionErrors int
	// Optional handler to log errors from Connect() and Wait()
	NotifyError func(error) error
	// Optional handler to log state changes. It can be used to block reconnection
	NotifyState func(ConnState)
}

// New initializes a reconnection struct
func New(c connection, params ...func(*Options)) Reconnect {
	opts := Options{}
	for _, fn := range params {
		fn(&opts)
	}
	r := reconnect{
		conn: c, opts: opts,
		closing: make(chan struct{}), closed: make(chan struct{}),
	}
	return &r
}

package reconnect

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

//go:generate mockery -name=connection -inpkg=true

func TestNew(t *testing.T) {
	c := &mockConnection{}
	New(c)
}

func TestNewWithOptions(t *testing.T) {
	c := &mockConnection{}
	New(c, func(o *Options) {
		o.MaxConnectAttempts = 1
	})
}

func TestCloseClosesUnderlyingConnection(t *testing.T) {
	c := &mockConnection{}
	c.On("Close").Return(nil)
	r := New(c)
	go r.Start()
	assert.NoError(t, r.Close())
	c.AssertCalled(t, "Close")
}

func TestCloseReturnsUnderlyingCloseError(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	c.On("Close").Return(err)
	r := New(c)
	go r.Start()
	assert.Error(t, r.Close())
}

func TestMaxConnectAttempts(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	c.On("Connect").Return(err).Times(3)
	r := New(c, func(o *Options) {
		o.MaxConnectAttempts = 3
	})
	assert.Error(t, r.Start())
}

func TestStart(t *testing.T) {
	c := &mockConnection{}
	w := make(chan time.Time)
	c.On("Connect").Return(nil).Once()
	c.On("Wait").Return(nil).WaitUntil(w)
	c.On("Close").Return(nil).Once()
	r := New(c)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		assert.NoError(t, r.Start())
		wg.Done()
	}()
	r.Close()
	close(w)
	wg.Wait()
}

func TestMaxConnectionErrors(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	c.On("Connect").Return(nil).Times(3)
	c.On("Wait").Return(err).Times(3)
	r := New(c, func(o *Options) {
		o.MaxConnectionErrors = 3
	})
	assert.Error(t, r.Start())
}

func TestNotifyErrorsNotifiesConnectErrors(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	calls := 0
	c.On("Connect").Return(err).Once()
	notification := func(err error) error {
		calls++
		assert.Error(t, err)
		return nil
	}
	r := New(c, func(o *Options) {
		o.NotifyError = notification
		o.MaxConnectAttempts = 1
	})
	assert.Error(t, r.Start())
	assert.Equal(t, calls, 1)
}

func TestNotifyErrorsNotifiesConnectionErrors(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	calls := 0
	c.On("Connect").Return(nil).Once()
	c.On("Wait").Return(err).Once()
	notification := func(err error) error {
		calls++
		assert.Error(t, err)
		return nil
	}
	r := New(c, func(o *Options) {
		o.NotifyError = notification
		o.MaxConnectionErrors = 1
	})
	assert.Error(t, r.Start())
	assert.Equal(t, calls, 1)
}

func TestNotifyErrorsStopsRetriesWhenErrorIsReturnedOnConnection(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	calls := 0
	c.On("Connect").Return(nil).Times(3)
	c.On("Wait").Return(err).Times(3)
	notification := func(err error) error {
		calls++
		assert.Error(t, err)
		if calls == 3 {
			return err
		}
		return nil
	}
	r := New(c, func(o *Options) {
		o.NotifyError = notification
		o.MaxConnectionErrors = 10
	})
	assert.Error(t, r.Start())
	assert.Equal(t, calls, 3)
}

func TestNotifyErrorsStopsRetriesWhenErrorIsReturnedConnecting(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	calls := 0
	c.On("Connect").Return(err).Times(3)
	notification := func(err error) error {
		calls++
		assert.Error(t, err)
		if calls == 3 {
			return err
		}
		return nil
	}
	r := New(c, func(o *Options) {
		o.NotifyError = notification
		o.MaxConnectionErrors = 10
	})
	assert.Error(t, r.Start())
	assert.Equal(t, calls, 3)
}

func TestEventLifecycleFailing(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	c.On("Connect").Return(err).Twice()
	c.On("Connect").Return(nil).Twice()
	c.On("Wait").Return(nil).Once()
	c.On("Wait").Return(err).Once()
	e := &lifecycleExpectation{}
	r := New(c, func(o *Options) {
		o.NotifyState = e.handler
		o.MaxConnectionErrors = 1
	})
	r.Start()
	e.Assert(t,
		StateConnecting,
		StateFailing, StateReconnecting,
		StateFailing, StateReconnecting,
		StateConnected, StateDisconnected,
		StateReconnecting, StateConnected,
		StateFailing, StateFailed,
	)
}

func TestEventLifecycleClosing(t *testing.T) {
	c := &mockConnection{}
	connected := make(chan time.Time)
	closing := make(chan time.Time)
	c.On("Connect").Return(nil).Once().Run(func(_ mock.Arguments) {
		close(connected)
	})
	c.On("Wait").Return(nil).Once().WaitUntil(closing)
	c.On("Close").Return(nil).Once().Run(func(_ mock.Arguments) {
		close(closing)
	})
	e := &lifecycleExpectation{}
	r := New(c, func(o *Options) {
		o.NotifyState = e.handler
		o.MaxConnectionErrors = 1
	})
	go func() {
		r.Start()
	}()
	<-connected
	r.Close()
	e.Assert(t,
		StateConnecting,
		StateConnected, StateDisconnected,
		StateClosed,
	)
}

func TestStringEvents(t *testing.T) {
	c := &mockConnection{}
	err := errors.New("fail")
	c.On("Connect").Return(err).Twice()
	c.On("Connect").Return(nil).Twice()
	c.On("Wait").Return(nil).Once()
	c.On("Wait").Return(err).Once()
	r := New(c, func(o *Options) {
		o.NotifyState = func(s ConnState) {}
		o.MaxConnectionErrors = 1
	})
	r.Start()
}

type lifecycleExpectation struct {
	result []ConnState
}

func (l *lifecycleExpectation) handler(s ConnState) {
	l.add(s)
}

func (l *lifecycleExpectation) add(s ConnState) {
	l.result = append(l.result, s)
}

func (l *lifecycleExpectation) Assert(t *testing.T, expected ...ConnState) {
	assert.Equal(t, expected, l.result)
}

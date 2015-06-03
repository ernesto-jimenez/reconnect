package reconnect

import "github.com/stretchr/testify/mock"

type mockConnection struct {
	mock.Mock
}

func (m *mockConnection) Connect() error {
	ret := m.Called()

	r0 := ret.Error(0)

	return r0
}
func (m *mockConnection) Wait() error {
	ret := m.Called()

	r0 := ret.Error(0)

	return r0
}
func (m *mockConnection) Close() error {
	ret := m.Called()

	r0 := ret.Error(0)

	return r0
}

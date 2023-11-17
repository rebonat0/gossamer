// Code generated by mockery v2.36.1. DO NOT EDIT.

package grandpa

import (
	mock "github.com/stretchr/testify/mock"
	constraints "golang.org/x/exp/constraints"
)

// HeaderBackendMock is an autogenerated mock type for the HeaderBackend type
type HeaderBackendMock[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	mock.Mock
}

type HeaderBackendMock_Expecter[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	mock *mock.Mock
}

func (_m *HeaderBackendMock[Hash, N, H]) EXPECT() *HeaderBackendMock_Expecter[Hash, N, H] {
	return &HeaderBackendMock_Expecter[Hash, N, H]{mock: &_m.Mock}
}

// ExpectBlockHashFromID provides a mock function with given fields: id
func (_m *HeaderBackendMock[Hash, N, H]) ExpectBlockHashFromID(id N) (Hash, error) {
	ret := _m.Called(id)

	var r0 Hash
	var r1 error
	if rf, ok := ret.Get(0).(func(N) (Hash, error)); ok {
		return rf(id)
	}
	if rf, ok := ret.Get(0).(func(N) Hash); ok {
		r0 = rf(id)
	} else {
		r0 = ret.Get(0).(Hash)
	}

	if rf, ok := ret.Get(1).(func(N) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// HeaderBackendMock_ExpectBlockHashFromID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ExpectBlockHashFromID'
type HeaderBackendMock_ExpectBlockHashFromID_Call[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	*mock.Call
}

// ExpectBlockHashFromID is a helper method to define mock.On call
//   - id N
func (_e *HeaderBackendMock_Expecter[Hash, N, H]) ExpectBlockHashFromID(id interface{}) *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H] {
	return &HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H]{Call: _e.mock.On("ExpectBlockHashFromID", id)}
}

func (_c *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H]) Run(run func(id N)) *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H] {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(N))
	})
	return _c
}

func (_c *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H]) Return(_a0 Hash, _a1 error) *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H] {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H]) RunAndReturn(run func(N) (Hash, error)) *HeaderBackendMock_ExpectBlockHashFromID_Call[Hash, N, H] {
	_c.Call.Return(run)
	return _c
}

// ExpectHeader provides a mock function with given fields: hash
func (_m *HeaderBackendMock[Hash, N, H]) ExpectHeader(hash Hash) (H, error) {
	ret := _m.Called(hash)

	var r0 H
	var r1 error
	if rf, ok := ret.Get(0).(func(Hash) (H, error)); ok {
		return rf(hash)
	}
	if rf, ok := ret.Get(0).(func(Hash) H); ok {
		r0 = rf(hash)
	} else {
		r0 = ret.Get(0).(H)
	}

	if rf, ok := ret.Get(1).(func(Hash) error); ok {
		r1 = rf(hash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// HeaderBackendMock_ExpectHeader_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ExpectHeader'
type HeaderBackendMock_ExpectHeader_Call[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	*mock.Call
}

// ExpectHeader is a helper method to define mock.On call
//   - hash Hash
func (_e *HeaderBackendMock_Expecter[Hash, N, H]) ExpectHeader(hash interface{}) *HeaderBackendMock_ExpectHeader_Call[Hash, N, H] {
	return &HeaderBackendMock_ExpectHeader_Call[Hash, N, H]{Call: _e.mock.On("ExpectHeader", hash)}
}

func (_c *HeaderBackendMock_ExpectHeader_Call[Hash, N, H]) Run(run func(hash Hash)) *HeaderBackendMock_ExpectHeader_Call[Hash, N, H] {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(Hash))
	})
	return _c
}

func (_c *HeaderBackendMock_ExpectHeader_Call[Hash, N, H]) Return(_a0 H, _a1 error) *HeaderBackendMock_ExpectHeader_Call[Hash, N, H] {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *HeaderBackendMock_ExpectHeader_Call[Hash, N, H]) RunAndReturn(run func(Hash) (H, error)) *HeaderBackendMock_ExpectHeader_Call[Hash, N, H] {
	_c.Call.Return(run)
	return _c
}

// Header provides a mock function with given fields: hash
func (_m *HeaderBackendMock[Hash, N, H]) Header(hash Hash) (*H, error) {
	ret := _m.Called(hash)

	var r0 *H
	var r1 error
	if rf, ok := ret.Get(0).(func(Hash) (*H, error)); ok {
		return rf(hash)
	}
	if rf, ok := ret.Get(0).(func(Hash) *H); ok {
		r0 = rf(hash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*H)
		}
	}

	if rf, ok := ret.Get(1).(func(Hash) error); ok {
		r1 = rf(hash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// HeaderBackendMock_Header_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Header'
type HeaderBackendMock_Header_Call[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	*mock.Call
}

// Header is a helper method to define mock.On call
//   - hash Hash
func (_e *HeaderBackendMock_Expecter[Hash, N, H]) Header(hash interface{}) *HeaderBackendMock_Header_Call[Hash, N, H] {
	return &HeaderBackendMock_Header_Call[Hash, N, H]{Call: _e.mock.On("Header", hash)}
}

func (_c *HeaderBackendMock_Header_Call[Hash, N, H]) Run(run func(hash Hash)) *HeaderBackendMock_Header_Call[Hash, N, H] {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(Hash))
	})
	return _c
}

func (_c *HeaderBackendMock_Header_Call[Hash, N, H]) Return(_a0 *H, _a1 error) *HeaderBackendMock_Header_Call[Hash, N, H] {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *HeaderBackendMock_Header_Call[Hash, N, H]) RunAndReturn(run func(Hash) (*H, error)) *HeaderBackendMock_Header_Call[Hash, N, H] {
	_c.Call.Return(run)
	return _c
}

// Info provides a mock function with given fields:
func (_m *HeaderBackendMock[Hash, N, H]) Info() Info[N] {
	ret := _m.Called()

	var r0 Info[N]
	if rf, ok := ret.Get(0).(func() Info[N]); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(Info[N])
	}

	return r0
}

// HeaderBackendMock_Info_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Info'
type HeaderBackendMock_Info_Call[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]] struct {
	*mock.Call
}

// Info is a helper method to define mock.On call
func (_e *HeaderBackendMock_Expecter[Hash, N, H]) Info() *HeaderBackendMock_Info_Call[Hash, N, H] {
	return &HeaderBackendMock_Info_Call[Hash, N, H]{Call: _e.mock.On("Info")}
}

func (_c *HeaderBackendMock_Info_Call[Hash, N, H]) Run(run func()) *HeaderBackendMock_Info_Call[Hash, N, H] {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *HeaderBackendMock_Info_Call[Hash, N, H]) Return(_a0 Info[N]) *HeaderBackendMock_Info_Call[Hash, N, H] {
	_c.Call.Return(_a0)
	return _c
}

func (_c *HeaderBackendMock_Info_Call[Hash, N, H]) RunAndReturn(run func() Info[N]) *HeaderBackendMock_Info_Call[Hash, N, H] {
	_c.Call.Return(run)
	return _c
}

// NewHeaderBackendMock creates a new instance of HeaderBackendMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewHeaderBackendMock[Hash constraints.Ordered, N constraints.Unsigned, H Header[Hash, N]](t interface {
	mock.TestingT
	Cleanup(func())
}) *HeaderBackendMock[Hash, N, H] {
	mock := &HeaderBackendMock[Hash, N, H]{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
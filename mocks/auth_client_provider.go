// Code generated by mockery v2.3.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// AuthClientProvider is an autogenerated mock type for the AuthClientProvider type
type AuthClientProvider struct {
	mock.Mock
}

// GetToken provides a mock function with given fields: env
func (_m *AuthClientProvider) GetToken(env string) (string, error) {
	ret := _m.Called(env)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(env)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(env)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
// Code generated by mockery v2.3.0. DO NOT EDIT.

package mocks

import (
	time "time"

	mock "github.com/stretchr/testify/mock"

	utils "github.com/LF-Engineering/da-ds/utils"
)

// ESClientProvider is an autogenerated mock type for the ESClientProvider type
type ESClientProvider struct {
	mock.Mock
}

// Add provides a mock function with given fields: index, documentID, body
func (_m *ESClientProvider) Add(index string, documentID string, body []byte) ([]byte, error) {
	ret := _m.Called(index, documentID, body)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(string, string, []byte) []byte); ok {
		r0 = rf(index, documentID, body)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, []byte) error); ok {
		r1 = rf(index, documentID, body)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Bulk provides a mock function with given fields: body
func (_m *ESClientProvider) Bulk(body []byte) ([]byte, error) {
	ret := _m.Called(body)

	var r0 []byte
	if rf, ok := ret.Get(0).(func([]byte) []byte); ok {
		r0 = rf(body)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]byte) error); ok {
		r1 = rf(body)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// BulkInsert provides a mock function with given fields: data
func (_m *ESClientProvider) BulkInsert(data []*utils.BulkData) ([]byte, error) {
	ret := _m.Called(data)

	var r0 []byte
	if rf, ok := ret.Get(0).(func([]*utils.BulkData) []byte); ok {
		r0 = rf(data)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]*utils.BulkData) error); ok {
		r1 = rf(data)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateIndex provides a mock function with given fields: index, body
func (_m *ESClientProvider) CreateIndex(index string, body []byte) ([]byte, error) {
	ret := _m.Called(index, body)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(string, []byte) []byte); ok {
		r0 = rf(index, body)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, []byte) error); ok {
		r1 = rf(index, body)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteIndex provides a mock function with given fields: index, ignoreUnavailable
func (_m *ESClientProvider) DeleteIndex(index string, ignoreUnavailable bool) ([]byte, error) {
	ret := _m.Called(index, ignoreUnavailable)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(string, bool) []byte); ok {
		r0 = rf(index, ignoreUnavailable)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, bool) error); ok {
		r1 = rf(index, ignoreUnavailable)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Get provides a mock function with given fields: index, query, result
func (_m *ESClientProvider) Get(index string, query map[string]interface{}, result interface{}) error {
	ret := _m.Called(index, query, result)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, map[string]interface{}, interface{}) error); ok {
		r0 = rf(index, query, result)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetStat provides a mock function with given fields: index, field, aggType, mustConditions, mustNotConditions
func (_m *ESClientProvider) GetStat(index string, field string, aggType string, mustConditions []map[string]interface{}, mustNotConditions []map[string]interface{}) (time.Time, error) {
	ret := _m.Called(index, field, aggType, mustConditions, mustNotConditions)

	var r0 time.Time
	if rf, ok := ret.Get(0).(func(string, string, string, []map[string]interface{}, []map[string]interface{}) time.Time); ok {
		r0 = rf(index, field, aggType, mustConditions, mustNotConditions)
	} else {
		r0 = ret.Get(0).(time.Time)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, string, []map[string]interface{}, []map[string]interface{}) error); ok {
		r1 = rf(index, field, aggType, mustConditions, mustNotConditions)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
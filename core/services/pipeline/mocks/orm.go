// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import (
	context "context"

	gorm "github.com/jinzhu/gorm"

	mock "github.com/stretchr/testify/mock"

	models "github.com/smartcontractkit/chainlink/core/store/models"

	pipeline "github.com/smartcontractkit/chainlink/core/services/pipeline"

	postgres "github.com/smartcontractkit/chainlink/core/services/postgres"

	time "time"
)

// ORM is an autogenerated mock type for the ORM type
type ORM struct {
	mock.Mock
}

// AwaitRun provides a mock function with given fields: ctx, runID
func (_m *ORM) AwaitRun(ctx context.Context, runID int64) error {
	ret := _m.Called(ctx, runID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, int64) error); ok {
		r0 = rf(ctx, runID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateRun provides a mock function with given fields: ctx, jobID, meta
func (_m *ORM) CreateRun(ctx context.Context, jobID int32, meta map[string]interface{}) (int64, error) {
	ret := _m.Called(ctx, jobID, meta)

	var r0 int64
	if rf, ok := ret.Get(0).(func(context.Context, int32, map[string]interface{}) int64); ok {
		r0 = rf(ctx, jobID, meta)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, int32, map[string]interface{}) error); ok {
		r1 = rf(ctx, jobID, meta)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateSpec provides a mock function with given fields: ctx, db, taskDAG, maxTaskTimeout
func (_m *ORM) CreateSpec(ctx context.Context, db *gorm.DB, taskDAG pipeline.TaskDAG, maxTaskTimeout models.Interval) (int32, error) {
	ret := _m.Called(ctx, db, taskDAG, maxTaskTimeout)

	var r0 int32
	if rf, ok := ret.Get(0).(func(context.Context, *gorm.DB, pipeline.TaskDAG, models.Interval) int32); ok {
		r0 = rf(ctx, db, taskDAG, maxTaskTimeout)
	} else {
		r0 = ret.Get(0).(int32)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *gorm.DB, pipeline.TaskDAG, models.Interval) error); ok {
		r1 = rf(ctx, db, taskDAG, maxTaskTimeout)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteRunsOlderThan provides a mock function with given fields: threshold
func (_m *ORM) DeleteRunsOlderThan(threshold time.Duration) error {
	ret := _m.Called(threshold)

	var r0 error
	if rf, ok := ret.Get(0).(func(time.Duration) error); ok {
		r0 = rf(threshold)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// FindBridge provides a mock function with given fields: name
func (_m *ORM) FindBridge(name models.TaskType) (models.BridgeType, error) {
	ret := _m.Called(name)

	var r0 models.BridgeType
	if rf, ok := ret.Get(0).(func(models.TaskType) models.BridgeType); ok {
		r0 = rf(name)
	} else {
		r0 = ret.Get(0).(models.BridgeType)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(models.TaskType) error); ok {
		r1 = rf(name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListenForNewRuns provides a mock function with given fields:
func (_m *ORM) ListenForNewRuns() (postgres.Subscription, error) {
	ret := _m.Called()

	var r0 postgres.Subscription
	if rf, ok := ret.Get(0).(func() postgres.Subscription); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(postgres.Subscription)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ProcessNextUnfinishedRun provides a mock function with given fields: ctx, fn
func (_m *ORM) ProcessNextUnfinishedRun(ctx context.Context, fn pipeline.ProcessRunFunc) (bool, error) {
	ret := _m.Called(ctx, fn)

	var r0 bool
	if rf, ok := ret.Get(0).(func(context.Context, pipeline.ProcessRunFunc) bool); ok {
		r0 = rf(ctx, fn)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, pipeline.ProcessRunFunc) error); ok {
		r1 = rf(ctx, fn)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ResultsForRun provides a mock function with given fields: ctx, runID
func (_m *ORM) ResultsForRun(ctx context.Context, runID int64) ([]pipeline.Result, error) {
	ret := _m.Called(ctx, runID)

	var r0 []pipeline.Result
	if rf, ok := ret.Get(0).(func(context.Context, int64) []pipeline.Result); ok {
		r0 = rf(ctx, runID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]pipeline.Result)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, int64) error); ok {
		r1 = rf(ctx, runID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RunFinished provides a mock function with given fields: runID
func (_m *ORM) RunFinished(runID int64) (bool, error) {
	ret := _m.Called(runID)

	var r0 bool
	if rf, ok := ret.Get(0).(func(int64) bool); ok {
		r0 = rf(runID)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(int64) error); ok {
		r1 = rf(runID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

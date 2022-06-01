package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsActiveProcess(t *testing.T) {
	srv := &Service{}
	srv.processes.Store(1, &process{DataID: 1, Current: PRC_BACKUP})

	testCases := []struct {
		Name         string
		Expected     bool
		CompareValue prc
	}{
		{
			Name:         "compare current BACKUP with matching",
			Expected:     true,
			CompareValue: PRC_BACKUP,
		},
		{
			Name:         "comparing current BACKUP with another",
			CompareValue: PRC_RESTORE,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.Expected, srv.isActiveProcess(tc.CompareValue))
		})
	}
}

func TestClearProcesses(t *testing.T) {
	srv := &Service{}
	srv.processes.Store(1, &process{DataID: 1, Current: PRC_BACKUP})
	prc, ok := srv.processes.Load(1)
	if !ok {
		assert.Error(t, errors.New("could not load value from processes map"))
	}
	p := prc.(*process)
	assert.Equal(t, PRC_BACKUP, p.Current)
	srv.clearProcesses()
	assert.Equal(t, false, srv.isActiveProcess(PRC_BACKUP))
}

func TestCurrentState(t *testing.T) {
	srv := &Service{
		state: STATE_ACTIVE,
	}
	srv.processes.Store(0, &process{DataID: 0, Current: PRC_RESTORE})
	// srv.processes.Store(1, &process{DataID: 1, Current: PRC_BACKUP})
	// srv.processes.Store(2, &process{DataID: 2, Current: PRC_RESTORE})
	assert.Equal(t,
		"Service is active with the following processes:[RESTORE]", srv.currentState())
}

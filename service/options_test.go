package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Comcast/webpa-common/logging"
	"github.com/strava/go.serversets"
	"github.com/stretchr/testify/assert"
)

func TestOptionsDefault(t *testing.T) {
	assert := assert.New(t)

	for _, o := range []*Options{nil, new(Options)} {
		t.Log(o)

		assert.NotNil(o.logger())
		assert.Equal([]string{DefaultServer}, o.servers())
		assert.Equal(DefaultConnectTimeout, o.connectTimeout())
		assert.Equal(DefaultSessionTimeout, o.sessionTimeout())
		assert.Equal(DefaultBaseDirectory, o.baseDirectory())
		assert.Equal(DefaultMemberPrefix, o.memberPrefix())
		assert.Equal(DefaultEnvironment, o.environment())
		assert.Equal(DefaultServiceName, o.serviceName())
		assert.Empty(o.registrations())
		assert.Equal(DefaultVnodeCount, o.vnodeCount())
		assert.Nil(o.pingFunc())
	}
}

func TestOptions(t *testing.T) {
	assert := assert.New(t)
	logger := logging.TestLogger(t)
	expectedError := errors.New("TestOptions expected error")
	testData := []struct {
		options                *Options
		expectedServers        []string
		expectedConnectTimeout time.Duration
		expectedSessionTimeout time.Duration
	}{
		{
			&Options{
				Logger:         logger,
				Servers:        []string{"node1.comcast.net:2181", "node2.comcast.net:275"},
				ConnectTimeout: 16 * time.Minute,
				SessionTimeout: 7 * time.Hour,
				BaseDirectory:  "/testOptions/workspace",
				MemberPrefix:   "testOptions_",
				Environment:    "test-options",
				ServiceName:    "options",
				Registrations:  []string{"https://comcast.net:8080"},
				VnodeCount:     67912723,
				PingFunc:       nil,
			},
			[]string{"node1.comcast.net:2181", "node2.comcast.net:275"},
			16 * time.Minute,
			7 * time.Hour,
		},
		{
			&Options{
				Connection:     "test1.comcast.net:2181",
				ConnectTimeout: -5 * time.Minute,
				SessionTimeout: -17 * time.Minute,
				BaseDirectory:  "/testOptions/workspace",
				MemberPrefix:   "testOptions_",
				Environment:    "test-options",
				ServiceName:    "options",
				VnodeCount:     34572,
				PingFunc:       func() error { return expectedError },
			},
			[]string{"test1.comcast.net:2181"},
			DefaultConnectTimeout,
			DefaultSessionTimeout,
		},
		{
			&Options{
				Connection:     "test1.comcast.net:2181, test2.foobar.com:9999   \t",
				Servers:        []string{"node1.qbert.net"},
				ConnectTimeout: 7 * time.Hour,
				BaseDirectory:  "/testOptions/workspace",
				MemberPrefix:   "testOptions_",
				Environment:    "test-options",
				ServiceName:    "options",
				VnodeCount:     34572,
				PingFunc:       func() error { return expectedError },
			},
			[]string{"test1.comcast.net:2181", "test2.foobar.com:9999", "node1.qbert.net"},
			7 * time.Hour,
			DefaultSessionTimeout,
		},
	}

	for _, record := range testData {
		t.Logf("%v", record)
		options := record.options

		if options.Logger != nil {
			assert.Equal(options.Logger, options.logger())
		} else {
			assert.NotNil(options.logger())
		}

		assert.Equal(record.expectedServers, options.servers())
		assert.Equal(record.expectedConnectTimeout, options.connectTimeout())
		assert.Equal(options.BaseDirectory, options.baseDirectory())
		assert.Equal(options.MemberPrefix, options.memberPrefix())
		assert.Equal(serversets.Environment(options.Environment), options.environment())
		assert.Equal(options.ServiceName, options.serviceName())
		assert.Equal(options.Registrations, options.registrations())
		assert.Equal(int(options.VnodeCount), options.vnodeCount())

		if options.PingFunc != nil {
			assert.Equal(expectedError, options.pingFunc()())
		} else {
			assert.Nil(options.pingFunc())
		}
	}
}

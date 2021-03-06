package fanout

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/tracing"
	"github.com/go-kit/kit/endpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNewNilSpanner(t *testing.T) {
	var (
		assert = assert.New(t)
		dummy  = func(context.Context, interface{}) (interface{}, error) {
			assert.Fail("The endpoint should not have been called")
			return nil, nil
		}
	)

	assert.Panics(func() {
		New(nil, map[string]endpoint.Endpoint{"test": dummy})
	})
}

func testNewNoConfiguredEndpoints(t *testing.T) {
	assert := assert.New(t)
	for _, empty := range []map[string]endpoint.Endpoint{nil, {}} {
		assert.Panics(func() {
			New(tracing.NewSpanner(), empty)
		})
	}
}

func testNewSuccessFirst(t *testing.T, serviceCount int) {
	var (
		require             = require.New(t)
		assert              = assert.New(t)
		logger              = logging.NewTestLogger(nil, t)
		expectedCtx, cancel = context.WithCancel(
			logging.WithLogger(context.Background(), logger),
		)

		expectedRequest  = "expectedRequest"
		expectedResponse = new(tracing.NopMergeable)

		endpoints   = make(map[string]endpoint.Endpoint, serviceCount)
		success     = make(chan string, 1)
		failureGate = make(chan struct{})
	)

	for i := 0; i < serviceCount; i++ {
		if i == 0 {
			endpoints["success"] = func(ctx context.Context, request interface{}) (interface{}, error) {
				assert.Equal(logger, logging.Logger(ctx))
				assert.Equal(expectedRequest, FromContext(ctx))
				assert.Equal(expectedRequest, request)
				success <- "success"
				return expectedResponse, nil
			}
		} else {
			endpoints[fmt.Sprintf("failure#%d", i)] = func(ctx context.Context, request interface{}) (interface{}, error) {
				assert.Equal(logger, logging.Logger(ctx))
				assert.Equal(expectedRequest, FromContext(ctx))
				assert.Equal(expectedRequest, request)
				<-failureGate
				return nil, fmt.Errorf("expected failure #%d", i)
			}
		}
	}

	defer cancel()
	fanout := New(tracing.NewSpanner(), endpoints)
	require.NotNil(fanout)

	response, err := fanout(expectedCtx, expectedRequest)
	assert.NoError(err)
	require.NotNil(response)
	assert.Equal("success", <-success)

	close(failureGate)
	spans := response.(tracing.Spanned).Spans()
	assert.Len(spans, 1)
	assert.Equal("success", spans[0].Name())
	assert.NoError(spans[0].Error())
}

func testNewSuccessLast(t *testing.T, serviceCount int) {
	var (
		require             = require.New(t)
		assert              = assert.New(t)
		logger              = logging.NewTestLogger(nil, t)
		expectedCtx, cancel = context.WithCancel(
			logging.WithLogger(context.Background(), logger),
		)

		expectedRequest  = "expectedRequest"
		expectedResponse = new(tracing.NopMergeable)

		endpoints    = make(map[string]endpoint.Endpoint, serviceCount)
		success      = make(chan string, 1)
		successGate  = make(chan struct{})
		failuresDone = new(sync.WaitGroup)
	)

	failuresDone.Add(serviceCount - 1)
	for i := 0; i < serviceCount; i++ {
		if i == 0 {
			endpoints["success"] = func(ctx context.Context, request interface{}) (interface{}, error) {
				assert.Equal(logger, logging.Logger(ctx))
				assert.Equal(expectedRequest, FromContext(ctx))
				assert.Equal(expectedRequest, request)
				<-successGate
				success <- "success"
				return expectedResponse, nil
			}
		} else {
			endpoints[fmt.Sprintf("failure#%d", i)] = func(ctx context.Context, request interface{}) (interface{}, error) {
				defer failuresDone.Done()
				assert.Equal(logger, logging.Logger(ctx))
				assert.Equal(expectedRequest, FromContext(ctx))
				assert.Equal(expectedRequest, request)
				return nil, fmt.Errorf("expected failure #%d", i)
			}
		}
	}

	defer cancel()
	fanout := New(tracing.NewSpanner(), endpoints)
	require.NotNil(fanout)

	// to force the success to be last, we spawn a goroutine to wait until
	// all failures are done followed by closing the success gate.
	go func() {
		failuresDone.Wait()
		close(successGate)
	}()

	response, err := fanout(expectedCtx, expectedRequest)
	assert.NoError(err)
	require.NotNil(response)
	assert.Equal("success", <-success)

	// because race detection and coverage can mess with the timings of select statements,
	// we have to allow a margin of error
	spans := response.(tracing.Spanned).Spans()
	assert.True(0 < len(spans) && len(spans) <= serviceCount)

	successSpanFound := false
	for _, s := range spans {
		if s.Name() == "success" {
			assert.NoError(s.Error())
			successSpanFound = true
		} else {
			assert.Error(s.Error())
		}
	}

	assert.True(successSpanFound)
}

func testNewTimeout(t *testing.T, serviceCount int) {
	var (
		require             = require.New(t)
		assert              = assert.New(t)
		logger              = logging.NewTestLogger(nil, t)
		expectedCtx, cancel = context.WithCancel(
			logging.WithLogger(context.Background(), logger),
		)

		expectedRequest  = "expectedRequest"
		expectedResponse = new(tracing.NopMergeable)

		endpoints        = make(map[string]endpoint.Endpoint, serviceCount)
		endpointGate     = make(chan struct{})
		endpointsWaiting = new(sync.WaitGroup)
	)

	endpointsWaiting.Add(serviceCount)
	for i := 0; i < serviceCount; i++ {
		endpoints[fmt.Sprintf("slow#%d", i)] = func(ctx context.Context, request interface{}) (interface{}, error) {
			assert.Equal(logger, logging.Logger(ctx))
			assert.Equal(expectedRequest, FromContext(ctx))
			assert.Equal(expectedRequest, request)
			endpointsWaiting.Done()
			<-endpointGate
			return expectedResponse, nil
		}
	}

	// release the endpoint goroutines when this test exits, to clean things up
	defer close(endpointGate)

	fanout := New(tracing.NewSpanner(), endpoints)
	require.NotNil(fanout)

	// in order to force a timeout in the select, we spawn a goroutine that waits until
	// all endpoints are blocked, then we cancel the context.
	go func() {
		endpointsWaiting.Wait()
		cancel()
	}()

	response, err := fanout(expectedCtx, expectedRequest)
	assert.Error(err)
	assert.Nil(response)

	spanError := err.(tracing.SpanError)
	assert.Equal(context.Canceled, spanError.Err())
	assert.Equal(context.Canceled.Error(), spanError.Error())
	assert.Empty(spanError.Spans())
}

func testNewAllEndpointsFail(t *testing.T, serviceCount int) {
	var (
		require             = require.New(t)
		assert              = assert.New(t)
		logger              = logging.NewTestLogger(nil, t)
		expectedCtx, cancel = context.WithCancel(
			logging.WithLogger(context.Background(), logger),
		)

		expectedRequest   = "expectedRequest"
		expectedLastError = fmt.Errorf("last error")

		endpoints          = make(map[string]endpoint.Endpoint, serviceCount)
		lastEndpointGate   = make(chan struct{})
		otherEndpointsDone = new(sync.WaitGroup)
	)

	otherEndpointsDone.Add(serviceCount - 1)
	for i := 0; i < serviceCount; i++ {
		if i == 0 {
			endpoints[fmt.Sprintf("failure#%d", i)] = func(ctx context.Context, request interface{}) (interface{}, error) {
				assert.Equal(logger, logging.Logger(ctx))
				assert.Equal(expectedRequest, FromContext(ctx))
				assert.Equal(expectedRequest, request)
				<-lastEndpointGate
				return nil, expectedLastError
			}
		} else {
			endpoints[fmt.Sprintf("failure#%d", i)] = func(index int) endpoint.Endpoint {
				return func(ctx context.Context, request interface{}) (interface{}, error) {
					defer otherEndpointsDone.Done()
					assert.Equal(logger, logging.Logger(ctx))
					assert.Equal(expectedRequest, FromContext(ctx))
					assert.Equal(expectedRequest, request)
					return nil, fmt.Errorf("failure#%d", index)
				}
			}(i)
		}
	}

	defer cancel()
	fanout := New(tracing.NewSpanner(), endpoints)
	require.NotNil(fanout)

	// in order to force a known endpoint to be last, we spawn a goroutine and wait
	// for the other, non-last endpoints to finish.  Then, we close the last endpoint gate.
	go func() {
		otherEndpointsDone.Wait()
		close(lastEndpointGate)
	}()

	response, err := fanout(expectedCtx, expectedRequest)
	assert.Error(err)
	assert.Nil(response)

	spanError := err.(tracing.SpanError)
	assert.Equal(expectedLastError, spanError.Err())
	assert.Equal(expectedLastError.Error(), spanError.Error())
	assert.Len(spanError.Spans(), serviceCount)
	for _, s := range spanError.Spans() {
		assert.Error(s.Error())
	}
}

func TestNew(t *testing.T) {
	t.Run("NoConfiguredEndpoints", testNewNoConfiguredEndpoints)
	t.Run("NilSpanner", testNewNilSpanner)

	t.Run("SuccessFirst", func(t *testing.T) {
		for c := 1; c <= 5; c++ {
			t.Run(fmt.Sprintf("EndpointCount=%d", c), func(t *testing.T) {
				testNewSuccessFirst(t, c)
			})
		}
	})

	t.Run("SuccessLast", func(t *testing.T) {
		for c := 1; c <= 5; c++ {
			t.Run(fmt.Sprintf("EndpointCount=%d", c), func(t *testing.T) {
				testNewSuccessLast(t, c)
			})
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		for c := 1; c <= 5; c++ {
			t.Run(fmt.Sprintf("EndpointCount=%d", c), func(t *testing.T) {
				testNewTimeout(t, c)
			})
		}
	})

	t.Run("AllEndpointsFail", func(t *testing.T) {
		for c := 1; c <= 5; c++ {
			t.Run(fmt.Sprintf("EndpointCount=%d", c), func(t *testing.T) {
				testNewAllEndpointsFail(t, c)
			})
		}
	})
}

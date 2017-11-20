package wrphttp

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"

	"github.com/Comcast/webpa-common/httperror"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/Comcast/webpa-common/wrp/wrpendpoint"
	gokithttp "github.com/go-kit/kit/transport/http"
)

// DecodeRequest is a go-kit DecodeRequestFunc that produces an Entity from the given HTTP request.
// The Content-Type header is used to determine the format, and if not specified wrp.Msgpack is used.
func DecodeRequest(ctx context.Context, original *http.Request) (interface{}, error) {
	contents, err := ioutil.ReadAll(original.Body)
	if err != nil {
		return nil, err
	}

	var format wrp.Format
	contentType := original.Header.Get("Content-Type")
	if len(contentType) == 0 {
		format = wrp.Msgpack
	} else {
		format, err = wrp.FormatFromContentType(contentType)
		if err != nil {
			return nil, err
		}
	}

	return wrp.DecodeEntityBytes(format, contents)
}

// DecodeRequestHeaders is a go-kit DecodeRequestFunc that uses the HTTP headers as fields of a WRP message.
// The HTTP entity, if specified, is used as the payload of the WRP message.
func DecodeRequestHeaders(ctx context.Context, original *http.Request) (interface{}, error) {
	payload, err := ioutil.ReadAll(original.Body)
	if err != nil {
		return nil, err
	}

	entity := Entity{
		Format: wrp.Msgpack,
	}

	err = SetMessageFromHeaders(original.Header, &entity.Message)
	if err != nil {
		return nil, err
	}

	entity.Message.Payload = payload
	return entity, nil
}

// ClientDecodeResponseBody produces a go-kit transport/http.DecodeResponseFunc that turns an HTTP response
// into a WRP response.
func ClientDecodeResponseBody(pool *wrp.DecoderPool) gokithttp.DecodeResponseFunc {
	return func(ctx context.Context, httpResponse *http.Response) (interface{}, error) {
		body, err := ioutil.ReadAll(httpResponse.Body)
		if err != nil {
			return nil, err
		}

		if httpResponse.StatusCode == http.StatusOK {
			var (
				contentType         = httpResponse.Header.Get("Content-Type")
				responseFormat, err = wrp.FormatFromContentType(contentType)
			)

			if err != nil {
				return nil, &httperror.E{Code: http.StatusBadRequest, Text: err.Error()}
			} else if responseFormat != pool.Format() {
				return nil, &httperror.E{Code: http.StatusUnsupportedMediaType, Text: fmt.Sprintf("Unexpected response Content-Type: %s", contentType)}
			}

			response, err := wrpendpoint.DecodeResponseBytes(body, pool)
			if err != nil {
				return nil, &httperror.E{Code: http.StatusInternalServerError, Text: err.Error()}
			}

			return response, nil
		}

		return nil, &httperror.E{Code: httpResponse.StatusCode}
	}
}

// ClientDecodeResponseHeaders is a go-kit transport/http.DecodeResponseFunc that turns an HTTP response
// formatted using headers for WRP fields into a WRP response.
func ClientDecodeResponseHeaders(ctx context.Context, httpResponse *http.Response) (interface{}, error) {
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}

	if httpResponse.StatusCode == http.StatusOK {
		message, err := NewMessageFromHeaders(httpResponse.Header, bytes.NewReader(body))
		if err != nil {
			return nil, &httperror.E{Code: http.StatusBadRequest, Text: err.Error()}
		}

		response, err := wrpendpoint.WrapAsResponse(message), nil
		if err != nil {
			return nil, &httperror.E{Code: http.StatusInternalServerError, Text: err.Error()}
		}

		return response, nil
	}

	return nil, &httperror.E{Code: httpResponse.StatusCode}
}

// withLogger enriches the given logger with request-specific information
func withLogger(logger log.Logger, r *http.Request) log.Logger {
	return log.WithPrefix(
		logger,
		"method", r.Method,
		"url", r.URL.String(),
		"protocol", r.Proto,
		"contentLength", r.ContentLength,
		"remoteAddress", r.RemoteAddr,
	)
}

// ServerDecodeRequestBody creates a go-kit transport/http.DecodeRequestFunc function that parses the body of an HTTP
// request as a WRP message in the format used by the given pool.  The supplied pool should match the
// Content-Type of the request, or an error is returned.
//
// This decoder function is appropriate when the HTTP request body contains a full WRP message.  For situations
// where the HTTP body is only the payload, use the Headers decoder.
func ServerDecodeRequestBody(logger log.Logger, pool *wrp.DecoderPool) gokithttp.DecodeRequestFunc {
	return func(ctx context.Context, httpRequest *http.Request) (interface{}, error) {
		return wrpendpoint.DecodeRequest(
			withLogger(logger, httpRequest),
			httpRequest.Body,
			pool,
		)
	}
}

// ServerDecodeRequestHeaders creates a go-kit transport/http.DecodeRequestFunc that builds a WRP request using HTTP
// headers for most message fields.  The HTTP entity body, if present, is used as the payload of the WRP message.
func ServerDecodeRequestHeaders(logger log.Logger) gokithttp.DecodeRequestFunc {
	return func(ctx context.Context, httpRequest *http.Request) (interface{}, error) {
		message, err := NewMessageFromHeaders(httpRequest.Header, httpRequest.Body)
		if err != nil {
			return nil, err
		}

		return wrpendpoint.WrapAsRequest(
			withLogger(logger, httpRequest),
			message,
		), nil
	}
}

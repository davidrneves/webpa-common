package fanouthttp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Comcast/webpa-common/middleware/fanout"
	"github.com/go-kit/kit/endpoint"
	gokithttp "github.com/go-kit/kit/transport/http"
)

// fanoutRequest is the internal type used to pass information to component requests.
// This type carries the original request so that downstream components can look at things
// like the header, the URL, etc.
type fanoutRequest struct {
	// original is the unmodified, original HTTP request passed to the fanout handler
	original *http.Request

	// relativeURL is the original URL with absolute fields removed, i.e. Scheme, Host, and User.
	relativeURL *url.URL

	// entity is the parsed HTTP entity returned by the configured DecodeRequestFunc
	entity interface{}
}

// Entity implements the fanout.Request interface.  This method allows access to the decoded HTTP entity that
// was parsed by the fanout's decoder.
func (fr *fanoutRequest) Entity() interface{} {
	return fr.entity
}

// decodeFanoutRequest is executed once per original request to turn an HTTP request into a fanoutRequest.
// The dec is used to perform one-time parsing on the original request to produce a custom entity object.
// If the dec function is nil, this function panics.
func decodeFanoutRequest(dec gokithttp.DecodeRequestFunc) gokithttp.DecodeRequestFunc {
	if dec == nil {
		panic("The entity decoder cannot be nil")
	}

	return func(ctx context.Context, original *http.Request) (interface{}, error) {
		entity, err := dec(ctx, original)
		if err != nil {
			return nil, err
		}

		relativeURL := *original.URL
		relativeURL.Scheme = ""
		relativeURL.Host = ""
		relativeURL.User = nil

		return &fanoutRequest{
			original:    original,
			relativeURL: &relativeURL,
			entity:      entity,
		}, nil
	}

}

// encodeComponentRequest creates the EncodeRequestFunc invoked for each component endpoint of a fanout.  Input to the
// return function is always a *fanoutRequest.  If the enc function is nil, this function panics.
func encodeComponentRequest(enc gokithttp.EncodeRequestFunc) gokithttp.EncodeRequestFunc {
	if enc == nil {
		panic("The entity encoder cannot be nil")
	}

	return func(ctx context.Context, component *http.Request, v interface{}) error {
		fanoutRequest := v.(*fanoutRequest)

		component.Method = fanoutRequest.original.Method
		component.URL = component.URL.ResolveReference(fanoutRequest.relativeURL)

		return enc(ctx, component, fanoutRequest.entity)
	}
}

// NewComponents producces a mapped set of go-kit endpoints, one for each supplied URL.  Each endpoint is expected to accept
// a fanoutRequest.  However, the encoder function is only expected to decode the HTTP entity.  The fanoutRequest is never passed
// to the supplied encoder function.
//
// This factory function is the approximate equivalent of go-kit's transport/http.NewClient.  In effect, it creates a multi-client.
// The resulting components can in turn be passed to fanout.New to create the aggregate fanout endpoint.
func NewComponents(urls []string, enc gokithttp.EncodeRequestFunc, dec gokithttp.DecodeResponseFunc, options ...gokithttp.ClientOption) (fanout.Components, error) {
	components := make(fanout.Components, len(urls))
	for _, raw := range urls {
		target, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}

		if len(target.Scheme) == 0 {
			return nil, fmt.Errorf("Endpoint '%s' does not specify a scheme", raw)
		}

		if len(target.RawQuery) > 0 {
			return nil, fmt.Errorf("Endpoint '%s' specifies a query string", raw)
		}

		// the method and target don't really matter, since they'll be replaced on each
		// request with the appropriate information from the original HTTP request.
		components[raw] = gokithttp.NewClient(
			"GET",
			target,
			encodeComponentRequest(enc),
			dec,
			options...,
		).Endpoint()
	}

	return components, nil
}

// NewHandler creates a fanout http.Handler that uses the specified endpoint.  The endpoint must have been
// returned by fanout.New or be a middleware decoration of the result from fanout.New.
//
// The decode request function is used to decode the component-specific request object.  Internally, a fanout request
// object is created that wraps the result of this function.
//
// The encode response function is used the encode the component-specific response object.  It is passed the same response
// object that comes from a successful fanout.Components endpoint.
func NewHandler(endpoint endpoint.Endpoint, dec gokithttp.DecodeRequestFunc, enc gokithttp.EncodeResponseFunc, options ...gokithttp.ServerOption) http.Handler {
	return gokithttp.NewServer(
		endpoint,
		decodeFanoutRequest(dec),
		enc,
		options...,
	)
}

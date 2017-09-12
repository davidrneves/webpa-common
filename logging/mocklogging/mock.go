package mocklogging

import (
	"github.com/stretchr/testify/mock"
)

// L is a mocked go-kit log.Logger for use in tests to verify logging behavior.
// This mock can be used directly, or set up with OnLog with is a bit more convenient.
type L struct {
	mock.Mock
}

func New() *L {
	return new(L)
}

// Log performs the standard mock behavior for the stretchr library
func (m *L) Log(keyvals ...interface{}) error {
	arguments := m.Called(keyvals)
	return arguments.Error(0)
}

// errorReporter describes the interface through with key/value matching errors are report.
// Both *testing.T and *testing.B implement this interface.
type errorReporter interface {
	Error(...interface{})
	Errorf(string, ...interface{})
}

// M returns a matcher function suitable to passed to mock.MatchedBy.
// The returns function always returns false if an odd number of objects
// were passed, i.e. it never matches any call in that case.
func M(e errorReporter, matches ...interface{}) func([]interface{}) bool {
	if len(matches)%2 != 0 {
		panic("odd number of matched key/value pairs")
	}

	return func(keyvals []interface{}) bool {
		if len(keyvals)%2 != 0 {
			e.Error("Odd number of logging key/value pairs")
			return false
		} else if len(matches) == 0 {
			// empty matches: always match
			return true
		}

		var foundExpected bool
		for i := 0; i < len(matches); i += 2 {
			expectedKey, expectedValue := matches[i], matches[i+1]

			foundExpected = false
			for j := 0; !foundExpected && j < len(keyvals); j += 2 {
				actualKey, actualValue := keyvals[j], keyvals[j+1]
				if actualKey != expectedKey {
					continue
				}

				foundExpected = true
				switch v := expectedValue.(type) {
				case func(interface{}) bool:
					if !v(actualValue) {
						e.Errorf("Key matcher for %s failed", expectedKey)
						return false
					}

				case func(interface{}, interface{}) bool:
					if !v(actualKey, actualValue) {
						e.Errorf("Key matcher for %s failed", expectedKey)
						return false
					}

				default:
					if expectedValue != actualValue {
						e.Errorf("Mismatched value for key %s: expected %s, but got %s", expectedKey, expectedValue, actualValue)
						return false
					}
				}
			}

			if !foundExpected {
				e.Errorf("Expected key %s was not present in the key/value pairs passed to Log", expectedKey)
				return false
			}
		}

		return true
	}
}

func anyValue(interface{}) bool {
	return true
}

// AnyValue returns a Log value matcher that matches any value.  This function is useful
// when you want to assert that a logging key is present but don't care what its value is.
// For example, OnLog(l, "aKey", AnyValue()) would expected "aKey" to be passed to Log with any value.
func AnyValue() func(interface{}) bool {
	return anyValue
}

// OnLog sets up a Log call with the given matches.  The call is returned
// for further customization, e.g. via Return or Once.
func OnLog(e errorReporter, l *L, matches ...interface{}) *mock.Call {
	return l.On("Log", mock.MatchedBy(M(e, matches...)))
}

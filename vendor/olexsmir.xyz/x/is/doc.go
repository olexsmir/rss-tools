// Package is provides minimal assertions for tests.
//
// Example:
//
//	func TestX(t *testing.T) {
//		var a = "Hello, Gopher!"
//		var b = "Hello, Gopher!"
//		is.Equal(t, a, b)
//
//		var err = errors.New("I'm an error!")
//		is.Err(t, err, nil)
//	}
package is

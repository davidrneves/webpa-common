// Code generated by "stringer -type=Type"; DO NOT EDIT.

package key

import "fmt"

const _Type_name = "HMACRSAEC"

var _Type_index = [...]uint8{0, 4, 7, 9}

func (i Type) String() string {
	if i < 0 || i >= Type(len(_Type_index)-1) {
		return fmt.Sprintf("Type(%d)", i)
	}
	return _Type_name[_Type_index[i]:_Type_index[i+1]]
}

/*
@Time : 2019-11-11 13:34
@Author : zr
*/
package errors

import (
	"fmt"
	"github.com/ZR233/goutils/errors2"
)

var (
	Unknown = errors2.U.NewErrorType(1, "unknown error", "unknown error")
	Socket  = errors2.U.NewErrorType(1, "socket error", "socket error")
	Header  = errors2.U.NewErrorType(2, "header error", "header error")
	Body    = errors2.U.NewErrorType(2, "header error", "header error")
)
var U = errors2.U

var (
	ErrRemoteDisconnect = fmt.Errorf("remote disconnect")
	ErrBodyLen          = fmt.Errorf("body len")
)

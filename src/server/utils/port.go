package utils

import (
	"fmt"
	"net"
	"strconv"
)

// PortCheck 检查端口是否可用，可用-true 不可用-false
func PortCheck(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%s", strconv.Itoa(port)))

	if err != nil {
		return true
	}
	defer l.Close()
	return false
}
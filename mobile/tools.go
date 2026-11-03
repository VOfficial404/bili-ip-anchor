//go:build tools

package mobile

// gomobile bind 要求模块依赖 golang.org/x/mobile/bind，这里让 go mod tidy 留着它
import _ "golang.org/x/mobile/bind"

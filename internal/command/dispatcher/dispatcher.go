package dispatcher

import (
	"github.com/andyj29/wannabet/internal/command"
	"github.com/andyj29/wannabet/internal/command/handler"
)

type Interface interface {
	Dispatch(p command.Interface) error
	RegisterHandler(command.Interface, handler.Interface) error
}

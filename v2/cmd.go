package v2

import "fmt"

type CmdInterface interface {
	Exec() error
}

type CmdClose struct {
	session *Session
}

func (c *CmdClose) Exec() error {
	return c.session.Close()
}

type CmdWrite struct {
	session *Session
	data    []byte
}

func (c *CmdWrite) Exec() error {
	n, err := c.session.Write(c.data)
	if err != nil {
		c.session.handleError(err)
		c.session.Close()
		return err
	}
	if n != len(c.data) {
		err = fmt.Errorf("data len %d, send len %d", len(c.data), n)
		c.session.handleError(err)
		c.session.Close()
	}

	return err
}

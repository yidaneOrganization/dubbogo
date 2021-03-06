/******************************************************
# DESC    : provide interface for rpc_steam;
#			encode app packet into byte stream by codec and send them to server by transport,
#           and then receive rsp stream and decode them into app package
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache Licence 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2016-06-30 10:45
# FILE    : rpc_codec.go
******************************************************/

package client

import (
	"bytes"
	"errors"
)

import (
	"github.com/AlexStocks/dubbogo/codec"
	"github.com/AlexStocks/dubbogo/codec/jsonrpc"
	"github.com/AlexStocks/dubbogo/transport"
)

const (
	lastStreamResponseError = "EOS"
)

// serverError represents an error that has been returned from
// the remote side of the RPC connection.
type serverError string

func (e serverError) Error() string {
	return string(e)
}

// errShutdown holds the specific error for closing/closed connections
var (
	errShutdown = errors.New("connection is shut down")
)

type rpcPlusCodec struct {
	client transport.Client
	codec  codec.Codec

	req *transport.Message
	buf *readWriteCloser
}

type readWriteCloser struct {
	wbuf *bytes.Buffer
	rbuf *bytes.Buffer
}

type clientCodec interface {
	WriteRequest(*request, interface{}) error
	ReadResponseHeader(*response) error
	ReadResponseBody(interface{}) error

	Close() error
}

type request struct {
	Service       string
	ServiceMethod string // format: "Service.Method"
	Seq           uint64 // sequence number chosen by client
	// next          *request // for free list in Server
}

type response struct {
	ServiceMethod string // echoes that of the Request
	Seq           uint64 // echoes that of the request
	Error         string // error, if any.
	// next          *response // for free list in Server
}

var (
	defaultContentType = "application/octet-stream"

	defaultCodecs = map[string]codec.NewCodec{
		"application/json":    jsonrpc.NewCodec,
		"application/jsonrpc": jsonrpc.NewCodec,
	}
)

func (rwc *readWriteCloser) Read(p []byte) (n int, err error) {
	return rwc.rbuf.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (n int, err error) {
	return rwc.wbuf.Write(p)
}

func (rwc *readWriteCloser) Close() error {
	rwc.rbuf.Reset()
	rwc.wbuf.Reset()
	return nil
}

func newRpcPlusCodec(req *transport.Message, client transport.Client, c codec.NewCodec) *rpcPlusCodec {
	rwc := &readWriteCloser{
		wbuf: bytes.NewBuffer(nil),
		rbuf: bytes.NewBuffer(nil),
	}
	r := &rpcPlusCodec{
		buf:    rwc,
		client: client,
		codec:  c(rwc),
		req:    req,
	}
	return r
}

func (c *rpcPlusCodec) WriteRequest(req *request, body interface{}) error {
	c.buf.wbuf.Reset()
	m := &codec.Message{
		Id:     req.Seq,
		Target: req.Service,
		Method: req.ServiceMethod,
		Type:   codec.Request,
		Header: map[string]string{},
	}
	if err := c.codec.Write(m, body); err != nil {
		return err
	}
	c.req.Body = c.buf.wbuf.Bytes()
	for k, v := range m.Header {
		c.req.Header[k] = v
	}
	return c.client.Send(c.req)
}

func (c *rpcPlusCodec) ReadResponseHeader(r *response) error {
	var m transport.Message
	if err := c.client.Recv(&m); err != nil {
		return err
	}
	c.buf.rbuf.Reset()
	c.buf.rbuf.Write(m.Body)
	var cm codec.Message
	err := c.codec.ReadHeader(&cm, codec.Response)
	r.ServiceMethod = cm.Method
	r.Seq = cm.Id
	r.Error = cm.Error
	return err
}

func (c *rpcPlusCodec) ReadResponseBody(b interface{}) error {
	return c.codec.ReadBody(b)
}

func (c *rpcPlusCodec) Close() error {
	// log.Debug("close rpcPlusCodec{%#v}", c)
	c.buf.Close()
	c.codec.Close()
	return c.client.Close()
}

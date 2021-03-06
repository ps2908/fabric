/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/peer"
	ehpb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var peerAddress = "0.0.0.0:60303"

type client struct {
	conn   *grpc.ClientConn
	stream peer.Events_ChatClient
}

func newClient() *client {
	conn, err := comm.NewClientConnectionWithAddress(peerAddress, true, false,
		nil, nil)
	if err != nil {
		panic(err)
	}

	stream, err := peer.NewEventsClient(conn).Chat(context.Background())
	if err != nil {
		panic(err)
	}

	cl := &client{
		conn:   conn,
		stream: stream,
	}
	go cl.processEvents()
	return cl
}

func (c *client) register(ies []*peer.Interest) error {
	emsg := &peer.Event{Event: &peer.Event_Register{Register: &peer.Register{Events: ies}}, Creator: signerSerialized}
	se, err := utils.GetSignedEvent(emsg, signer)
	if err != nil {
		return err
	}
	return c.stream.Send(se)
}

func (c *client) unregister(ies []*peer.Interest) error {
	emsg := &peer.Event{Event: &peer.Event_Unregister{Unregister: &peer.Unregister{Events: ies}}, Creator: signerSerialized}
	se, err := utils.GetSignedEvent(emsg, signer)
	if err != nil {
		return err
	}
	return c.stream.Send(se)
}

func (c *client) processEvents() error {
	defer c.stream.CloseSend()
	for {
		_, err := c.stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestEvents(t *testing.T) {
	test := func(duration time.Duration) {
		t.Log(duration)
		f := func() {
			Send(nil)
		}
		assert.Panics(t, f)
		Send(&peer.Event{})
		gEventProcessorBck := gEventProcessor
		gEventProcessor = nil
		e, err := createRegisterEvent(nil, nil)
		assert.NoError(t, err)
		Send(e)
		gEventProcessor = gEventProcessorBck
		Send(e)
	}
	prevTimeout := gEventProcessor.Timeout
	for _, timeout := range []time.Duration{0, -1, 1} {
		gEventProcessor.Timeout = timeout
		test(timeout)
	}
	gEventProcessor.Timeout = prevTimeout
}

func TestDeregister(t *testing.T) {
	f := func() {
		gEventProcessor.deregisterHandler(nil, nil)
	}
	assert.Panics(t, f)
	assert.Error(t, gEventProcessor.deregisterHandler(&peer.Interest{EventType: 100}, nil))
	assert.NoError(t, gEventProcessor.deregisterHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, nil))
}

func TestRegisterHandler(t *testing.T) {
	f := func() {
		gEventProcessor.registerHandler(nil, nil)
	}
	assert.Panics(t, f)

	// attempt to register handlers (invalid type or nil handlers)
	assert.Error(t, gEventProcessor.registerHandler(&peer.Interest{EventType: 100}, nil))
	assert.Error(t, gEventProcessor.registerHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, nil))
	assert.Error(t, gEventProcessor.registerHandler(&peer.Interest{EventType: peer.EventType_CHAINCODE}, nil))

	// attempt to register valid handler
	m := newMockEventhub()
	defer close(m.recvChan)
	handler := newHandler(m, gEventProcessor)
	assert.NoError(t, handler.eventProcessor.registerHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, handler))

	// clean up by deregistering handler
	assert.NoError(t, handler.eventProcessor.deregisterHandler(&peer.Interest{EventType: peer.EventType_BLOCK}, handler))
}

func TestProcessEvents(t *testing.T) {
	c := newClient()
	interests := []*peer.Interest{
		{EventType: peer.EventType_BLOCK},
	}
	c.register(interests)
	e, err := createRegisterEvent(nil, nil)
	assert.NoError(t, err)
	go Send(e)
	c.unregister(interests)
}

func TestInitializeEvents_twice(t *testing.T) {
	config := &EventsServerConfig{
		BufferSize:  100,
		Timeout:     0,
		SendTimeout: 0,
		TimeWindow:  0,
	}
	initializeEventsTwice := func() {
		initializeEvents(config)
	}
	assert.Panics(t, initializeEventsTwice)
}

func TestAddEventType_alreadyDefined(t *testing.T) {
	assert.Error(t, gEventProcessor.addEventType(ehpb.EventType_BLOCK), "BLOCK type already defined")
}

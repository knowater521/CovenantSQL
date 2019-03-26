package rpc

import (
	"bytes"
	"net"

	"github.com/pkg/errors"

	"github.com/CovenantSQL/CovenantSQL/crypto/etls"
	"github.com/CovenantSQL/CovenantSQL/crypto/hash"
	"github.com/CovenantSQL/CovenantSQL/crypto/kms"
	"github.com/CovenantSQL/CovenantSQL/pow/cpuminer"
	"github.com/CovenantSQL/CovenantSQL/proto"
)

const (
	// ETLSHeaderSize is the header size with ETLSHeader + NodeID + Nonce
	ETLSHeaderSize = 2 + hash.HashBSize + 32
)

type ETLSConn struct {
	*etls.CryptoConn
	isClient bool

	// The following fields may be rewritten during handshake.
	isAnonymous  bool
	remoteNodeID proto.RawNodeID
}

func NewServerConn(conn net.Conn) *ETLSConn {
	return &ETLSConn{
		CryptoConn: etls.NewConnWithRaw(conn),
	}
}

func NewClientConn(conn net.Conn) *ETLSConn {
	return &ETLSConn{
		CryptoConn: etls.NewConnWithRaw(conn),
		isClient:   true,
	}
}

func (c *ETLSConn) RemoteNodeID() proto.RawNodeID {
	return c.remoteNodeID
}

func (c *ETLSConn) Handshake() (err error) {
	if c.isClient {
		return c.clientHandshake()
	}
	return c.serverHandshake()
}

func (c *ETLSConn) serverHandshake() (err error) {
	headerBuf := make([]byte, ETLSHeaderSize)
	rCount, err := c.CryptoConn.Conn.Read(headerBuf)
	if err != nil {
		err = errors.Wrap(err, "read node header error")
		return
	}

	if rCount != ETLSHeaderSize {
		err = errors.New("invalid ETLS header size")
		return
	}

	if !bytes.Equal(headerBuf[:2], etls.ETLSMagicBytes) {
		err = errors.New("bad ETLS header")
		return
	}

	// headerBuf len is hash.HashBSize, so there won't be any error
	idHash, _ := hash.NewHash(headerBuf[2 : 2+hash.HashBSize])
	rawNodeID := &proto.RawNodeID{Hash: *idHash}
	// TODO(auxten): compute the nonce and check difficulty
	cpuminer.Uint256FromBytes(headerBuf[2+hash.HashBSize:])

	isAnonymous := rawNodeID.IsEqual(&kms.AnonymousRawNodeID.Hash)
	symmetricKey, err := GetSharedSecretWith(rawNodeID, isAnonymous)
	if err != nil {
		err = errors.Wrapf(err, "get shared secret, target: %s", rawNodeID.String())
		return
	}
	cipher := etls.NewCipher(symmetricKey)
	c.CryptoConn.Cipher = cipher
	c.remoteNodeID = *rawNodeID
	c.isAnonymous = isAnonymous

	return
}

func (c *ETLSConn) clientHandshake() (err error) {
	writeBuf := make([]byte, ETLSHeaderSize)
	writeBuf[0] = etls.ETLSMagicBytes[0]
	writeBuf[1] = etls.ETLSMagicBytes[1]
	if c.isAnonymous {
		copy(writeBuf[2:], kms.AnonymousRawNodeID.AsBytes())
		copy(writeBuf[2+hash.HashSize:], (&cpuminer.Uint256{}).Bytes())
	} else {
		// send NodeID + Uint256 Nonce
		var nodeIDBytes []byte
		var nonce *cpuminer.Uint256
		nodeIDBytes, err = kms.GetLocalNodeIDBytes()
		if err != nil {
			err = errors.Wrap(err, "get local node id failed")
			return
		}
		nonce, err = kms.GetLocalNonce()
		if err != nil {
			err = errors.Wrap(err, "get local nonce failed")
			return
		}
		copy(writeBuf[2:2+hash.HashSize], nodeIDBytes)
		copy(writeBuf[2+hash.HashSize:], nonce.Bytes())
	}
	wrote, err := c.Conn.Write(writeBuf)
	if err != nil {
		err = errors.Wrap(err, "write node id and nonce failed")
		return
	}

	if wrote != ETLSHeaderSize {
		err = errors.Errorf("write header size not match %d", wrote)
		return
	}
	return
}

// dialToNode connects to the node with nodeID.
func cDialToNode(nodeID proto.NodeID) (conn net.Conn, err error) {
	return cDialToNodeEx(nodeID, false)
}

// DialToNode ties use connection in pool, if fails then connects to the node with nodeID.
func DialToNodeWithPool(nodeID proto.NodeID, pool *SessionPool, isAnonymous bool) (conn net.Conn, err error) {
	if pool == nil || isAnonymous {
		conn, err = cDialToNodeEx(nodeID, isAnonymous)
		return
	}
	//log.WithField("poolSize", pool.Len()).Debug("session pool size")
	conn, err = pool.Get(nodeID)
	return
}

// dialToNodeEx connects to the node with nodeID.
func cDialToNodeEx(nodeID proto.NodeID, isAnonymous bool) (conn net.Conn, err error) {
	var rawNodeID = nodeID.ToRawNodeID()
	/*
		As a common practice of PKI, we should add some randomness to the ECDHed pre-master-key
		we did that at the [ETLS](../crypto/etls) layer with a non-deterministic authenticated
		encryption input vector(random IV).

		To understand that and func "rpc.GetSharedSecretWith"
		Please refer to:
			- RFC 5297: Synthetic Initialization Vector (SIV) Authenticated Encryption
				Using the Advanced Encryption Standard (AES)
			- RFC 5246: The Transport Layer Security (TLS) Protocol Version 1.2
		Useful links:
			- https://tools.ietf.org/html/rfc5297#section-3
			- https://tools.ietf.org/html/rfc5246#section-5
			- https://www.cryptologie.net/article/340/tls-pre-master-secrets-and-master-secrets/
	*/
	symmetricKey, err := GetSharedSecretWith(rawNodeID, isAnonymous)
	if err != nil {
		return
	}

	nodeAddr, err := GetNodeAddr(rawNodeID)
	if err != nil {
		err = errors.Wrapf(err, "resolve %s failed", rawNodeID.String())
		return
	}

	cipher := etls.NewCipher(symmetricKey)
	conn, err = cdial("tcp", nodeAddr, rawNodeID, cipher, isAnonymous)
	if err != nil {
		err = errors.Wrapf(err, "connect %s %s failed", rawNodeID.String(), nodeAddr)
		return
	}

	return
}

// dial connects to a address with a Cipher
// address should be in the form of host:port.
func cdial(network, address string, remoteNodeID *proto.RawNodeID, cipher *etls.Cipher, isAnonymous bool) (c *ETLSConn, err error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		err = errors.Wrapf(err, "connect to node %s failed", address)
		return
	}

	c = &ETLSConn{
		CryptoConn:   etls.NewConnEx(conn, cipher),
		isAnonymous:  isAnonymous,
		isClient:     true,
		remoteNodeID: *remoteNodeID,
	}

	if err = c.Handshake(); err != nil {
		return
	}

	return
}

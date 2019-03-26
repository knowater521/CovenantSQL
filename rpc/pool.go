/*
 * Copyright 2018 The CovenantSQL Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rpc

import (
	"net"
	"sync"

	"github.com/pkg/errors"

	"github.com/CovenantSQL/CovenantSQL/conf"
	"github.com/CovenantSQL/CovenantSQL/proto"
)

// NodeDialer is the dialer handler.
type NodeDialer func(nodeID proto.NodeID) (net.Conn, error)

// SessionMap is the map from node id to session.
type SessionMap map[proto.NodeID]*Session

// Session is the Session type of SessionPool.
type Session struct {
	sync.RWMutex
	nodeDialer NodeDialer
	target     proto.NodeID
	sess       chan net.Conn
}

// SessionPool is the struct type of session pool.
type SessionPool struct {
	sync.RWMutex
	sessions   SessionMap
	nodeDialer NodeDialer
}

var (
	instance *SessionPool
	once     sync.Once
)

// Close closes the session.
func (s *Session) Close() {
	s.Lock()
	defer s.Unlock()
	close(s.sess)
	for s := range s.sess {
		_ = s.Close()
	}
}

// Get returns new connection from session.
func (s *Session) Get() (sess net.Conn, err error) {
	s.Lock()
	defer s.Unlock()

	if sess, ok := <-s.sess; ok {
		return sess, nil
	}
	return s.newSession()
}

// Len returns physical connection count.
func (s *Session) Len() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.sess)
}

func (s *Session) newSession() (conn net.Conn, err error) {
	conn, err = s.nodeDialer(s.target)
	if err != nil {
		err = errors.Wrap(err, "dialing new session connection failed")
		return
	}

	return
}

func (s *Session) put(conn net.Conn) (ok bool) {
	s.Lock()
	defer s.Unlock()
	select {
	case s.sess <- conn:
	default:
		_ = conn.Close()
	}
	return
}

// newSessionPool creates a new SessionPool.
func newSessionPool(nd NodeDialer) *SessionPool {
	return &SessionPool{
		sessions:   make(SessionMap),
		nodeDialer: nd,
	}
}

// GetSessionPoolInstance return default SessionPool instance with rpc.DefaultDialer.
func GetSessionPoolInstance() *SessionPool {
	once.Do(func() {
		instance = newSessionPool(DefaultDialer)
	})
	return instance
}

func (p *SessionPool) getSession(id proto.NodeID) (sess *Session, loaded bool) {
	// NO Blocking operation in this function
	p.Lock()
	defer p.Unlock()
	sess, exist := p.sessions[id]
	if exist {
		//log.WithField("node", id).Debug("load session for target node")
		loaded = true
	} else {
		// new session
		sess = &Session{
			nodeDialer: p.nodeDialer,
			target:     id,
			sess:       make(chan net.Conn, conf.MaxRPCPoolPhysicalConnection),
		}
		p.sessions[id] = sess
	}
	return
}

// Get returns existing session to the node, if not exist try best to create one.
func (p *SessionPool) Get(id proto.NodeID) (conn net.Conn, err error) {
	var sess *Session
	sess, _ = p.getSession(id)
	return sess.Get()
}

func (p *SessionPool) Put(id proto.NodeID, conn net.Conn) (ok bool) {
	p.Lock()
	defer p.Unlock()
	sess, ok := p.sessions[id]
	if ok {
		sess.put(conn)
	}
	return
}

// Remove the node sessions in the pool.
func (p *SessionPool) Remove(id proto.NodeID) {
	p.Lock()
	defer p.Unlock()
	sess, exist := p.sessions[id]
	if exist {
		sess.Close()
		delete(p.sessions, id)
	}
	return
}

// Close closes all sessions in the pool.
func (p *SessionPool) Close() {
	p.Lock()
	defer p.Unlock()
	for _, s := range p.sessions {
		s.Close()
	}
	p.sessions = make(SessionMap)
}

// Len returns the session counts in the pool.
func (p *SessionPool) Len() (total int) {
	p.RLock()
	defer p.RUnlock()

	for _, s := range p.sessions {
		total += s.Len()
	}
	return
}

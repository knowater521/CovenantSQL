/*
 * Copyright 2018 The ThunderDB Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the “License”);
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an “AS IS” BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package blockproducer

import (
	"gitlab.com/thunderdb/ThunderDB/blockproducer/types"
	"gitlab.com/thunderdb/ThunderDB/kayak"
	"gitlab.com/thunderdb/ThunderDB/proto"
	"gitlab.com/thunderdb/ThunderDB/rpc"
	"time"
)

const (
	blockVersion int32 = 0x01
)

// config is the main chain configuration
type config struct {
	genesis *types.Block

	dataFile string

	server *rpc.Server

	peers      *kayak.Peers
	nodeID 	proto.NodeID

	period  time.Duration
	tick    time.Duration
}

// newConfig creates new config
func newConfig(genesis *types.Block, dataFile string,
	server *rpc.Server, peers *kayak.Peers,
	nodeID proto.NodeID, period time.Duration, tick time.Duration) *config {
	config := config{
		genesis: genesis,
		dataFile: dataFile,
		server: server,
		peers: peers,
		nodeID: nodeID,
		period: period,
		tick: tick,
	}
	return &config
}

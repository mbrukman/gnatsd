// Copyright 2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !race

package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

// IMPORTANT: Tests in this file are not executed when running with the -race flag.

func TestNoRaceRouteSendLocalSubs(t *testing.T) {
	template := `
			port: -1
			write_deadline: "3s"
			cluster {
				port: -1		
				%s
			}
	`
	cfa := createConfFile(t, []byte(fmt.Sprintf(template, "")))
	defer os.Remove(cfa)
	srvA, optsA := RunServerWithConfig(cfa)
	defer srvA.Shutdown()

	cfb := createConfFile(t, []byte(fmt.Sprintf(template, "")))
	srvB, optsB := RunServerWithConfig(cfb)
	defer srvB.Shutdown()

	clientA := createClientConn(t, optsA.Host, optsA.Port)
	defer clientA.Close()

	clientASend, clientAExpect := setupConn(t, clientA)
	clientASend("PING\r\n")
	clientAExpect(pongRe)

	clientB := createClientConn(t, optsB.Host, optsB.Port)
	defer clientB.Close()

	clientBSend, clientBExpect := setupConn(t, clientB)
	clientBSend("PING\r\n")
	clientBExpect(pongRe)

	// total number of subscriptions per server
	totalPerServer := 60000
	for i := 0; i < totalPerServer; i++ {
		proto := fmt.Sprintf("SUB foo.%d %d\r\n", i, i*2)
		clientASend(proto)
		clientBSend(proto)
	}
	clientASend("PING\r\n")
	clientAExpect(pongRe)
	clientBSend("PING\r\n")
	clientBExpect(pongRe)

	checkExpectedSubs(totalPerServer, srvA, srvB)

	routes := fmt.Sprintf(`
		routes: [
			"nats://%s:%d"
		]
	`, optsA.Cluster.Host, optsA.Cluster.Port)
	if err := ioutil.WriteFile(cfb, []byte(fmt.Sprintf(template, routes)), 0600); err != nil {
		t.Fatalf("Error rewriting B's config file: %v", err)
	}
	if err := srvB.Reload(); err != nil {
		t.Fatalf("Error on reload: %v", err)
	}
	// The rid should be the current CID+1.
	connz, _ := srvB.Connz(nil)
	expectedRID := connz.Conns[0].Cid + 1

	checkClusterFormed(t, srvA, srvB)
	checkExpectedSubs(2*totalPerServer, srvA, srvB)

	// Conditions above could still pass but there could be
	// multiple route reconnect, so we check that route
	// connected only once.
	checkRoute := func(t *testing.T) {
		t.Helper()
		routez, _ := srvB.Routez(nil)
		if len(routez.Routes) != 1 {
			t.Fatalf("Expected 1 route, got %v", len(routez.Routes))
		}
		if r := routez.Routes[0]; r.Rid != expectedRID {
			t.Fatalf("Expected route to connect only once and have rid=%v, got rid=%v", expectedRID, r.Rid)
		}
	}
	checkRoute(t)
}

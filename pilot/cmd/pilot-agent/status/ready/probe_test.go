// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ready

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	admin "github.com/envoyproxy/go-control-plane/envoy/admin/v2alpha"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/gomega"
)

var (
	goodStats      = "cluster_manager.cds.update_success: 1\nlistener_manager.lds.update_success: 1"
	liveServerInfo = &admin.ServerInfo{State: admin.ServerInfo_LIVE}
	initServerInfo = &admin.ServerInfo{State: admin.ServerInfo_INITIALIZING}
)

func TestEnvoyStatsCompleteAndSuccessful(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := goodStats

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).NotTo(HaveOccurred())
}

func TestEnvoyStatsIncompleteCDS(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "listener_manager.lds.update_success: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cds updates: 0"))
}

func TestEnvoyStatsIncompleteLDS(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "cluster_manager.cds.update_success: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("lds updates: 0"))
}

func TestEnvoyStatsCompleteAndRejectedCDS(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "cluster_manager.cds.update_rejected: 1\nlistener_manager.lds.update_success: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).NotTo(HaveOccurred())
}

func TestEnvoyStatsCompleteAndRejectedLDS(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "cluster_manager.cds.update_success: 1\nlistener_manager.lds.update_rejected: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).NotTo(HaveOccurred())
}

func TestEnvoyCheckFailsIfStatsUnparsableNoSeparator(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "cluster_manager.cds.update_success; 1\nlistener_manager.lds.update_success: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("missing separator"))
}

func TestEnvoyCheckFailsIfStatsUnparsableNoNumber(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := "cluster_manager.cds.update_success: a\nlistener_manager.lds.update_success: 1"

	server := createAndStartServer(stats, liveServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed parsing Envoy stat"))
}

func TestEnvoyCheckSucceedsIfStatsCleared(t *testing.T) {
	g := NewGomegaWithT(t)
	probe := Probe{AdminPort: 1234}

	// Verify bad stats trigger an error
	badStats := "cluster_manager.cds.update_success: 0\nlistener_manager.lds.update_success: 0"
	server := createAndStartServer(badStats, liveServerInfo)
	err := probe.Check()
	server.Close()
	g.Expect(err).To(HaveOccurred())

	// trigger the state change
	server = createAndStartServer(goodStats, liveServerInfo)
	err = probe.Check()
	server.Close()
	g.Expect(err).NotTo(HaveOccurred())

	// verify empty stats no longer break probe
	server = createAndStartServer(badStats, liveServerInfo)
	err = probe.Check()
	server.Close()
	g.Expect(err).ToNot(HaveOccurred())
}

func TestEnvoyInitializing(t *testing.T) {
	g := NewGomegaWithT(t)
	stats := goodStats

	server := createAndStartServer(stats, initServerInfo)
	defer server.Close()
	probe := Probe{AdminPort: 1234}

	err := probe.Check()

	g.Expect(err).To(HaveOccurred())
}

func createAndStartServer(statsToReturn string, serverInfo proto.Message) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Send response to be tested
		rw.Write([]byte(statsToReturn))
	}))
	mux.HandleFunc("/server_info", http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		jsonm := &jsonpb.Marshaler{Indent: "  "}
		infoJSON, _ := jsonm.MarshalToString(serverInfo)

		// Send response to be tested
		rw.Write([]byte(infoJSON))
	}))

	// Start a local HTTP server
	server := httptest.NewUnstartedServer(mux)

	l, err := net.Listen("tcp", "127.0.0.1:1234")
	if err != nil {
		panic("Could not create listener for test: " + err.Error())
	}
	server.Listener = l
	server.Start()
	return server
}

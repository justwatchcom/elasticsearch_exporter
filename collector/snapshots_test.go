// Copyright 2021 The Prometheus Authors
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

package collector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestSnapshots(t *testing.T) {
	// Testcases created using $REPO/hack/snapshots_test_setup.sh

	tcs := map[string][]string{
		"5.6.16": {`{"succeed":{"type":"fs","settings":{"location":"/tmp/succeed"}},"fail":{"type":"fs","settings":{"location":"/tmp/fail"}}}`, `{"snapshots":[{"snapshot":"visible","uuid":"A1oKg_8PQBKzuRkQOOMG-w","version_id":5061699,"version":"5.6.16","indices":["foo_1","foo_2"],"state":"SUCCESS","start_time":"2020-04-23T12:18:26.872Z","start_time_in_millis":1587644306872,"end_time":"2020-04-23T12:18:27.052Z","end_time_in_millis":1587644307052,"duration_in_millis":180,"failures":[],"shards":{"total":2,"failed":0,"successful":2}}]}`, `{"error":{"root_cause":[{"type":"repository_exception","reason":"[fail] could not read repository data from index blob"}],"type":"repository_exception","reason":"[fail] could not read repository data from index blob","caused_by":{"type":"access_denied_exception","reason":"/tmp/fail"}},"status":500}`},
		"6.8.8":  {`{"succeed":{"type":"fs","settings":{"location":"/tmp/succeed"}},"fail":{"type":"fs","settings":{"location":"/tmp/fail"}}}`, `{"snapshots":[{"snapshot":"visible","uuid":"-fCjIUhqTZG9GVlBZPEI3A","version_id":6080899,"version":"6.8.8","indices":["foo_1","foo_2"],"include_global_state":true,"state":"SUCCESS","start_time":"2020-04-23T12:18:35.681Z","start_time_in_millis":1587644315681,"end_time":"2020-04-23T12:18:35.925Z","end_time_in_millis":1587644315925,"duration_in_millis":244,"failures":[],"shards":{"total":2,"failed":0,"successful":2}}]}`, `{"error":{"root_cause":[{"type":"repository_exception","reason":"[fail] could not read repository data from index blob"}],"type":"repository_exception","reason":"[fail] could not read repository data from index blob","caused_by":{"type":"access_denied_exception","reason":"/tmp/fail"}},"status":500}`},
		"7.6.2":  {`{"succeed":{"type":"fs","settings":{"location":"/tmp/succeed"}},"fail":{"type":"fs","settings":{"location":"/tmp/fail"}}}`, `{"snapshots":[{"snapshot":"visible","uuid":"ON95BFtaRjaQPaHEnhYdnA","version_id":7050299,"version":"7.5.2","indices":["foo_1","foo_2"],"include_global_state":true,"state":"SUCCESS","start_time":"2020-04-23T12:18:46.550Z","start_time_in_millis":1587644326550,"end_time":"2020-04-23T12:18:46.950Z","end_time_in_millis":1587644326950,"duration_in_millis":400,"failures":[],"shards":{"total":2,"failed":0,"successful":2}}]}`, `{"error":{"root_cause":[{"type":"repository_exception","reason":"[fail] Could not determine repository generation from root blobs"}],"type":"repository_exception","reason":"[fail] Could not determine repository generation from root blobs","caused_by":{"type":"access_denied_exception","reason":"/tmp/fail"}},"status":500}`},
	}
	for ver, out := range tcs {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.RequestURI {
			case "/_snapshot":
				fmt.Fprint(w, out[0])
				return
			case "/_snapshot/succeed/_all":
				fmt.Fprint(w, out[1])
				return
			case "/_snapshot/fail/_all":
				fmt.Fprint(w, out[2])
				return
			}
		}))
		defer ts.Close()

		u, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("Failed to parse URL: %s", err)
		}
		s := NewSnapshots(log.NewNopLogger(), http.DefaultClient, u)
		stats, err := s.fetchAndDecodeSnapshotsStats()
		if err != nil {
			t.Fatalf("Failed to fetch or decode snapshots stats: %s", err)
		}
		t.Logf("[%s] Snapshots Response: %+v", ver, stats)

		for repo, snapshotResponse := range stats {
			if repo == "fail" {
				if snapshotResponse.Snapshots != nil {
					t.Errorf("Returning non-nil Snapshots response for inaccessible repo")
				}
				continue
			} else {
				if len(snapshotResponse.Snapshots[0].Indices) != 2 {
					t.Errorf("Bad number of snapshot indices")
				}
				if len(snapshotResponse.Snapshots[0].Failures) != int(snapshotResponse.Snapshots[0].Shards.Failed) {
					t.Errorf("Bad number of snapshot failures")
				}
				if snapshotResponse.Snapshots[0].Shards.Total != 2 {
					t.Errorf("Bad number of snapshot shards total")
				}
				if snapshotResponse.Snapshots[0].Shards.Successful != 2 {
					t.Errorf("Bad number of snapshot shards successful")
				}
				if len(snapshotResponse.Snapshots) != 1 {
					t.Errorf("Bad number of repository snapshots")
				}
			}
		}
	}

}

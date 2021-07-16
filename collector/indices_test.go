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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestIndices(t *testing.T) {
	// Testcases created using:
	//  docker run -d -p 9200:9200 elasticsearch:VERSION-alpine
	//  curl -XPUT http://localhost:9200/foo_1/type1/1 -d '{"title":"abc","content":"hello"}'
	//  curl -XPUT http://localhost:9200/foo_1/type1/2 -d '{"title":"def","content":"world"}'
	//  curl -XPUT http://localhost:9200/foo_2/type1/1 -d '{"title":"abc001","content":"hello001"}'
	//  curl -XPUT http://localhost:9200/foo_2/type1/2 -d '{"title":"def002","content":"world002"}'
	//  curl -XPUT http://localhost:9200/foo_2/type1/3 -d '{"title":"def003","content":"world003"}'
	//  curl http://localhost:9200/_all/_stats
	files := []string{
		"../fixtures/indices-1.7.7.json",
		"../fixtures/indices-2.4.5.json",
		"../fixtures/indices-5.4.2.json",
		"../fixtures/indices-5.5.2.json",
	}
	const failMsgFormat = "%v, expected %v, actual %v, testfile: %s"
	for _, filename := range files {
		f, _ := os.Open(filename)
		defer f.Close()

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(w, f)
		}))
		defer ts.Close()

		u, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("Failed to parse URL: %s", err)
		}
		i := NewIndices(log.NewNopLogger(), http.DefaultClient, u, false)
		stats, err := i.fetchAndDecodeIndexStats()
		if err != nil {
			t.Fatalf("Failed to fetch or decode indices stats: %s", err)
		}
		//t.Logf("[%s] Index Response: %+v", filename, stats)
		if stats.Indices["foo_1"].Primaries.Docs.Count != 2 {
			t.Errorf("Wrong number of primary docs")
		}
		if stats.Indices["foo_1"].Primaries.Store.SizeInBytes == 0 {
			t.Errorf("Wrong number of primary store size in bytes")
		}
		if stats.Indices["foo_1"].Total.Store.SizeInBytes == 0 {
			t.Errorf("Wrong number of total store size in bytes")
		}
		if stats.Indices["foo_1"].Total.Indexing.IndexTimeInMillis == 0 {
			t.Errorf("Wrong indexing time recorded")
		}
		if stats.Indices["foo_1"].Total.Indexing.IndexTotal == 0 {
			t.Errorf("Wrong indexing total recorded")
		}
		if stats.Shards.Failed != 0 {
			t.Errorf(failMsgFormat, "Failed shards count wrong", 0, stats.Shards.Failed, filename)
		}
		if stats.Shards.Total != 20 {
			t.Errorf(failMsgFormat, "Total shard count wrong", 0, stats.Shards.Failed, filename)
		}
		if stats.Shards.Successful != 10 {
			t.Errorf(failMsgFormat, "Successful shards count wrong", 0, stats.Shards.Failed, filename)
		}
	}
}

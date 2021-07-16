#!/bin/bash

function get_test_case {

  local es_version; es_version=$1
  local containerID; containerID=$2
  local doctype; doctype=$3

  # Wait for ES to become available
  while ! curl http://localhost:9200/_cluster/health >/dev/null 2>&1; do
    sleep 2
  done

  # Create indices (made these explicit since defaults have changed since 5.4.2)
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_1" -d '{"settings" : {"number_of_shards" : 1, "number_of_replicas" : 1}}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_2" -d '{"settings" : {"number_of_shards" : 1, "number_of_replicas" : 1}}' >/dev/null

  # Create events
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_1/$doctype/1" -d '{"title":"abc","content":"hello"}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_1/$doctype/2" -d '{"title":"def","content":"world"}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_2/$doctype/1" -d '{"title":"abc001","content":"hello001"}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_2/$doctype/2" -d '{"title":"def002","content":"world002"}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/foo_2/$doctype/3" -d '{"title":"def003","content":"world003"}' >/dev/null

  # Create snapshot repositories
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/_snapshot/succeed" -d '{"type": "fs","settings":{"location": "/tmp/succeed"}}' >/dev/null
  curl -s -H 'Content-Type: application/json' -XPUT "http://localhost:9200/_snapshot/fail" -d '{"type": "fs","settings":{"location": "/tmp/fail"}}' >/dev/null

  # Create snapshots
  curl -s -XPUT "http://localhost:9200/_snapshot/succeed/visible?wait_for_completion=true" >/dev/null
  curl -s -XPUT "http://localhost:9200/_snapshot/fail/notvisible?wait_for_completion=true" >/dev/null

  # Remove access to the "fail" repo
  docker exec -it "$containerID" chmod 000 /tmp/fail

  # Echo out test case responses
  # shellcheck disable=SC2016
  echo "\"$es_version\": " '{`'"$(curl -s http://localhost:9200/_snapshot)"'`, `'"$(curl -s http://localhost:9200/_snapshot/succeed/_all)"'`, `'"$(curl -s http://localhost:9200/_snapshot/fail/_all)"'`},'

  # Kill docker container
  docker kill "$containerID" >/dev/null
}

get_test_case 5.6.16 "$(docker run -d --rm -p 9200:9200 -e xpack.security.enabled=false -e discovery.type=single-node -e path.repo=/tmp docker.elastic.co/elasticsearch/elasticsearch:5.6.16)" "type1"
get_test_case 6.8.8 "$(docker run -d --rm -p 9200:9200 -e xpack.security.enabled=false -e discovery.type=single-node -e path.repo=/tmp docker.elastic.co/elasticsearch/elasticsearch:6.8.8)" "type1"
get_test_case 7.6.2 "$(docker run -d --rm -p 9200:9200 -e discovery.type=single-node -e path.repo=/tmp docker.elastic.co/elasticsearch/elasticsearch:7.5.2)" "_doc"

package collector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestMapping(t *testing.T) {
	// Testcases created using:
	//  docker run -p 9200:9200 -e "discovery.type=single-node" elasticsearch:7.8.0
	//  curl -XPUT http://localhost:9200/twitter
	//  curl -XPUT http://localhost:9200/facebook
	/*  curl -XPUT http://localhost:9200/twitter/_mapping -H 'Content-Type: application/json' -d'{
	    "properties": {
	        "email": {
	            "type": "keyword"
	        },
	        "phone": {
	            "type": "keyword"
	        }
	    }
	}'*/
	/*  curl -XPUT http://localhost:9200/facebook/_mapping -H 'Content-Type: application/json' -d'{
	    "properties": {
	        "name": {
	            "type": "text",
	            "fields": {
	                "raw": {
	                    "type": "keyword"
	                }
	            }
	        },
	        "contact": {
	            "properties": {
	                "email": {
	                    "type": "text",
	                    "fields": {
	                        "raw": {
	                            "type": "keyword"
	                        }
	                    }
	                },
	                "phone": {
	                    "type": "text"
	                }
	            }
	        }
	    }
	}'*/
	//  curl http://localhost:9200/_all/_mapping
	tcs := map[string]string{
		"7.8.0": `{
			"facebook": {
			  "mappings": {
				"properties": {
				  "contact": {
					"properties": {
					  "email": {
						"type": "text",
						"fields": {
						  "raw": {
							"type": "keyword"
						  }
						}
					  },
					  "phone": {
						"type": "text"
					  }
					}
				  },
				  "name": {
					"type": "text",
					"fields": {
					  "raw": {
						"type": "keyword"
					  }
					}
				  }
				}
			  }
			},
			"twitter": {
			  "mappings": {
				"properties": {
				  "email": {
					"type": "keyword"
				  },
				  "phone": {
					"type": "keyword"
				  }
				}
			  }
			}
		  }`,
	}
	for ver, out := range tcs {
		for hn, handler := range map[string]http.Handler{
			"plain": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, out)
			}),
		} {
			ts := httptest.NewServer(handler)
			defer ts.Close()

			u, err := url.Parse(ts.URL)
			if err != nil {
				t.Fatalf("Failed to parse URL: %s", err)
			}
			c := NewIndicesMappings(log.NewNopLogger(), http.DefaultClient, u)
			imr, err := c.fetchAndDecodeIndicesMappings()
			if err != nil {
				t.Fatalf("Failed to fetch or decode indices mappings: %s", err)
			}
			t.Logf("[%s/%s] All Indices Mappings Response: %+v", hn, ver, imr)

			response := *imr
			if *response["facebook"].Mappings.Properties["contact"].Properties["phone"].Type != "text" {
				t.Errorf("Marshalling error at facebook.contact.phone")
			}

			if *response["facebook"].Mappings.Properties["contact"].Properties["email"].Fields["raw"].Type != "keyword" {
				t.Errorf("Marshalling error at facebook.contact.email.raw")
			}

			if *response["facebook"].Mappings.Properties["name"].Type != "text" {
				t.Errorf("Marshalling error at facebook.name")
			}

			if *response["facebook"].Mappings.Properties["name"].Fields["raw"].Type != "keyword" {
				t.Errorf("Marshalling error at facebook.name.raw")
			}

			if *response["twitter"].Mappings.Properties["email"].Type != "keyword" {
				t.Errorf("Marshalling error at twitter.email")
			}

			if *response["twitter"].Mappings.Properties["phone"].Type != "keyword" {
				t.Errorf("Marshalling error at twitter.phone")
			}

		}
	}
}

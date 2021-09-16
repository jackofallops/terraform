package jsonprovider

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/hashicorp/terraform/internal/terraform"
)

type allSchemas struct {
	tfSchemas *terraform.Schemas
}

type providerSchema struct {
	Name   string
	Schema *terraform.ProviderSchema
}

type itemType string

const (
	ItemTypeDataSource itemType = "datasource"
	ItemTypeResource   itemType = "resource"
	ItemTypeNone       itemType = ""
)

type Route struct {
	Name         string           `json:"name"`
	Method       string           `json:"method"`
	Pattern      string           `json:"pattern"`
	Queries      []string         `json:"-"`
	ProviderData providerSchema   `json:"-"`
	HandlerFunc  http.HandlerFunc `json:"-"`
}

type Routes []Route

// set some basic routes

type queryParams struct {
	prettyPrint bool
}

var routes = Routes{
	Route{
		Name:    "provider",
		Method:  http.MethodGet,
		Pattern: "/",
	},
	Route{
		Name:        "health",
		Method:      http.MethodGet,
		Pattern:     "/health",
		HandlerFunc: healthCheck,
	},
}

func StartAPI(d *terraform.Schemas) {
	data := allSchemas{tfSchemas: d}
	router := data.NewRouter()

	log.Fatal(http.ListenAndServe(":8081", router))
}

func (p allSchemas) serveAllProviders(w http.ResponseWriter, _ *http.Request) {
	output, err := Marshal(p.tfSchemas)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("something went wrong Marshalling: %+v", err)))
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Add("Data-Size", fmt.Sprintf("data is length %d", len(output)))
	w.WriteHeader(http.StatusOK)

	// Just dump everything we know about for right now, this is not helpful but shows we're able to get data out...
	if err := json.NewEncoder(w).Encode(string(output)); err != nil {
		panic(err)
	}
}

func (t providerSchema) serveProvider(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	vars := mux.Vars(r)
	name, hasName := vars["name"]
	switch len(pathParts) {
	case 3, 4:
		switch strings.ToLower(pathParts[2]) {
		case "resource":
			if !hasName {
				w.Header().Set("Content-Type", "application/json; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(t.Schema.ResourceTypes); err != nil {
					panic(err)
				}
			} else {
				w.Header().Set("Content-Type", "application/json; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(t.Schema.ResourceTypes[name]); err != nil {
					panic(err)
				}
			}

		case "datasource":
			if !hasName {
				w.Header().Set("Content-Type", "application/json; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(t.Schema.DataSources); err != nil {
					panic(err)
				}
			} else {
				w.Header().Set("Content-Type", "application/json; charset=UTF-8")
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(t.Schema.DataSources[name]); err != nil {
					panic(err)
				}
			}

		default:
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.WriteHeader(http.StatusNotImplemented)
			w.Write([]byte(""))
		}
	default:
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(t); err != nil {
			panic(err)
		}
	}

}

func (r Routes) listAllRoutes(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	q := processQueryParams(req)
	if q.prettyPrint {
		prettyOut, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("couldn't unmarshall data for pretty print of routes: %+v", err)))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(prettyOut)
	} else {
		jsonRoutes, err := json.Marshal(r)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("failed to marshal routes: %+v", err)))
		}
		w.WriteHeader(http.StatusOK)
		w.Write(jsonRoutes)
	}
}

func (p *allSchemas) NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	routes.buildRoutesFromProviders(p.tfSchemas)
	for _, route := range routes {
		var handler http.Handler
		if route.HandlerFunc == nil {
			route.HandlerFunc = route.ProviderData.serveProvider
		}
		handler = route.HandlerFunc

		router.Methods(route.Method).Path(route.Pattern).Name(route.Name).Handler(handler)

	}
	return router
}

func (r *Routes) buildRoutesFromProviders(data *terraform.Schemas) {
	for k, v := range data.Providers {
		*r = append(*r, Route{
			Name:    k.Type,
			Method:  http.MethodGet,
			Pattern: decorateNameForRoute(k.Namespace, k.Type, ItemTypeNone, true),
			ProviderData: providerSchema{
				Name:   k.Type,
				Schema: v,
			},
		})
		if len(v.DataSources) > 0 {
			*r = append(*r, Route{
				Name:    k.Type,
				Method:  http.MethodGet,
				Pattern: decorateNameForRoute(k.Namespace, k.Type, ItemTypeDataSource, true),
				ProviderData: providerSchema{
					Name:   k.Type,
					Schema: v,
				},
			})
			*r = append(*r, Route{
				Name:    k.Type,
				Method:  http.MethodGet,
				Pattern: decorateNameForRoute(k.Namespace, k.Type, ItemTypeDataSource, false),
				ProviderData: providerSchema{
					Name:   k.Type,
					Schema: v,
				},
			})
		}
		if len(v.ResourceTypes) > 0 {
			*r = append(*r, Route{
				Name:    k.Type,
				Method:  http.MethodGet,
				Pattern: decorateNameForRoute(k.Namespace, k.Type, ItemTypeResource, true),
				ProviderData: providerSchema{
					Name:   k.Type,
					Schema: v,
				},
			})
			*r = append(*r, Route{
				Name:    k.Type,
				Method:  http.MethodGet,
				Pattern: decorateNameForRoute(k.Namespace, k.Type, ItemTypeResource, false),
				ProviderData: providerSchema{
					Name:   k.Type,
					Schema: v,
				},
			})
		}
	}
	*r = append(*r, Route{
		Name:        "routeList",
		Method:      http.MethodGet,
		Pattern:     "/routes",
		HandlerFunc: r.listAllRoutes,
	})
}

func decorateNameForRoute(namespace, name string, i itemType, list bool) (route string) {
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		name = parts[len(parts)]
	}

	route = fmt.Sprintf("/%s/%s", namespace, name)
	if i != ItemTypeNone {
		route = fmt.Sprintf("%s/%s", route, string(i))
	}
	if !list {
		route = fmt.Sprintf("%s/{name}", route)
	}
	//return fmt.Sprintf("/%s/%s", namespace, name)
	return
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("[{\"status\": \"Server running and routing\"}]")))
}

func processQueryParams(req *http.Request) (result queryParams) {
	q := req.URL.Query()
	if _, ok := q["pretty"]; ok {
		result.prettyPrint = true
	}

	return
}

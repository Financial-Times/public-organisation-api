package organisations

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/service-status-go/gtg"
	"github.com/gorilla/mux"
)

// OrganisationDriver for cypher queries
var OrganisationDriver Driver
var CacheControlHeader string

// HealthCheck does something
func HealthCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "neo4j-check",
		BusinessImpact:   "Unable to respond to Public Organisations api requests",
		Name:             "Check connectivity to Neo4j",
		PanicGuide:       "https://dewey.ft.com/public-org-api.html",
		Severity:         2,
		TechnicalSummary: "Cannot connect to Neo4j a instance with at least one organisation loaded in it",
		Checker:          Checker,
	}
}

// Checker does more stuff
func Checker() (string, error) {
	err := OrganisationDriver.CheckConnectivity()
	if err == nil {
		return "Connectivity to neo4j is ok", err
	}
	return "Error connecting to neo4j", err
}

// Ping says pong
func Ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

// BuildInfoHandler - This is a stop gap and will be added to when we can define what we should display here
func BuildInfoHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "build-info")
}

// MethodNotAllowedHandler does stuff
func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	return
}

const validUUID = "([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$"

// GetOrganisation is the public API
func GetOrganisation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if uuid == "" {
		http.Error(w, "uuid required", http.StatusBadRequest)
		return
	}
	organisation, found, err := OrganisationDriver.Read(uuid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "` + err.Error() + `"}`))
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Organisation not found."}`))
		return
	}
	//if the request was not made for the canonical, but an alternate uuid: redirect
	if !strings.Contains(organisation.ID, uuid) {
		validRegexp := regexp.MustCompile(validUUID)
		canonicalUUID := validRegexp.FindString(organisation.ID)
		redirectURL := strings.Replace(r.RequestURI, uuid, canonicalUUID, 1)
		w.Header().Set("Location", redirectURL)
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}

	w.Header().Set("Cache-Control", CacheControlHeader)
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(organisation)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"Organisation could not be marshelled, err=` + err.Error() + `"}`))
	}
}

//GoodToGo returns a 503 if the healthcheck fails - suitable for use from varnish to check availability of a node
func GTG() gtg.Status {
	statusCheck := func() gtg.Status {
		return gtgCheck(Checker)
	}
	return gtg.FailFastParallelCheck([]gtg.StatusChecker{statusCheck})()
}

func gtgCheck(handler func() (string, error)) gtg.Status {
	if _, err := handler(); err != nil {
		return gtg.Status{GoodToGo: false, Message: err.Error()}
	}
	return gtg.Status{GoodToGo: true}
}

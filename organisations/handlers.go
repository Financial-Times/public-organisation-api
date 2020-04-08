package organisations

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	logger "github.com/Financial-Times/go-logger"
	"github.com/Financial-Times/neo-model-utils-go/mapper"
	"github.com/Financial-Times/service-status-go/gtg"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type HTTPClient interface {
	Do(req *http.Request) (resp *http.Response, err error)
}

type OrganisationsHandler struct {
	client      HTTPClient
	conceptsURL string
}

// OrganisationDriver for cypher queries
var CacheControlHeader string

const (
	validUUID           = "([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$"
	ontologyPrefix      = "http://www.ft.com/ontology"
	organisationSuffix  = "/organisation/Organisation"
	publicCompanySuffix = "/company/PublicCompany"
	relatedQueryParam   = "?showRelationship=related"
	isParentPredicate   = "/parentOrganisationOf"
	hasParentPredicate  = "/subOrganisationOf"
	issuedPredicate     = "/issued"
	thingsApiUrl        = "http://api.ft.com/things/"
	ftThing             = "http://www.ft.com/thing/"
)

func NewHandler(client HTTPClient, conceptsURL string) OrganisationsHandler {
	return OrganisationsHandler{
		client,
		conceptsURL,
	}
}

func (h *OrganisationsHandler) RegisterHandlers(router *mux.Router) {
	logger.Info("Registering handlers")
	mh := handlers.MethodHandler{
		"GET": http.HandlerFunc(h.GetOrganisation),
	}

	path := "/organisations/{uuid}"
	router.Handle(path, mh)
	router.HandleFunc(path, h.MethodNotAllowedHandler)
}

// HealthCheck does something
func (h *OrganisationsHandler) HealthCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "public-concepts-api-check",
		BusinessImpact:   "Unable to respond to Public Organisations api requests",
		Name:             "Check connectivity to public-concepts-api",
		PanicGuide:       "https://runbooks.in.ft.com/public-org-api",
		Severity:         2,
		TechnicalSummary: "Not being able to communicate with public-concepts-api means that requests for organisations cannot be performed.",
		Checker:          h.Checker,
	}
}

// Checker does more stuff
func (h *OrganisationsHandler) Checker() (string, error) {
	req, err := http.NewRequest("GET", h.conceptsURL+"/__gtg", nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("User-Agent", "UPP public-organisations-api")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("health check returned a non-200 HTTP status: %v", resp.StatusCode)
	}
	return "Public Concepts API is healthy", nil

}

// Ping says pong
func Ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong")
}

// BuildInfoHandler - This is a stop gap and will be added to when we can define what we should display here
func (h *OrganisationsHandler) BuildInfoHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "build-info")
}

// MethodNotAllowedHandler does stuff
func (h *OrganisationsHandler) MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	return
}

// GetOrganisation is the public API
func (h *OrganisationsHandler) GetOrganisation(w http.ResponseWriter, r *http.Request) {
	uuidMatcher := regexp.MustCompile(validUUID)
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	transID := transactionidutils.GetTransactionIDFromRequest(r)

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if uuid == "" || !uuidMatcher.MatchString(uuid) {
		msg := fmt.Sprintf(`uuid '%s' is either missing or invalid`, uuid)
		logger.WithTransactionID(transID).WithUUID(uuid).Error(msg)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "` + msg + `"}`))
		return
	}

	organisation, found, err := h.getOrganisationViaConceptsAPI(uuid, transID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "failed to return organisation"}`))
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "organisation not found"}`))
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
func (h *OrganisationsHandler) GTG() gtg.Status {
	statusCheck := func() gtg.Status {
		return gtgCheck(h.Checker)
	}
	return gtg.FailFastParallelCheck([]gtg.StatusChecker{statusCheck})()
}

func gtgCheck(handler func() (string, error)) gtg.Status {
	if _, err := handler(); err != nil {
		return gtg.Status{GoodToGo: false, Message: err.Error()}
	}
	return gtg.Status{GoodToGo: true}
}

func (h *OrganisationsHandler) getOrganisationViaConceptsAPI(uuid string, transID string) (organisation Organisation, found bool, err error) {
	org := Organisation{}

	reqURL := h.conceptsURL + "/concepts/" + uuid + relatedQueryParam

	request, err := http.NewRequest("GET", reqURL, nil)

	if err != nil {
		msg := fmt.Sprintf("failed to create request to %s", reqURL)
		logger.WithError(err).WithUUID(uuid).WithTransactionID(transID).Error(msg)
		return org, false, err
	}

	request.Header.Set("X-Request-Id", transID)
	resp, err := h.client.Do(request)
	if err != nil {
		msg := fmt.Sprintf("request to %s was unsuccessful", reqURL)
		logger.WithError(err).WithUUID(uuid).WithTransactionID(transID).Error(msg)
		return org, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return org, false, nil
	}

	conceptsApiResponse := ConceptApiResponse{}
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		msg := fmt.Sprintf("failed to read response body: %v", resp.Body)
		logger.WithError(err).WithUUID(uuid).WithTransactionID(transID).Error(msg)
		return org, false, err
	}

	if err = json.Unmarshal(body, &conceptsApiResponse); err != nil {
		msg := fmt.Sprintf("failed to unmarshal response body: %v", body)
		logger.WithError(err).WithUUID(uuid).WithTransactionID(transID).Error(msg)
		return org, false, err
	}

	if conceptsApiResponse.Type != ontologyPrefix+organisationSuffix && conceptsApiResponse.Type != ontologyPrefix+publicCompanySuffix {
		logger.WithTransactionID(transID).WithUUID(uuid).Info("requested concept is not a organisation")
		return org, false, nil
	}

	org.ID = convertID(conceptsApiResponse.ID)
	org.APIURL = convertApiUrl(conceptsApiResponse.ApiURL, "organisations")
	org.PrefLabel = conceptsApiResponse.PrefLabel
	org.Types = mapper.FullTypeHierarchy(conceptsApiResponse.Type)
	org.DirectType = conceptsApiResponse.Type
	org.PostalCode = conceptsApiResponse.PostalCode
	org.CountryCode = conceptsApiResponse.CountryCode
	org.CountryOfIncorporation = conceptsApiResponse.CountryOfIncorporation
	org.LegalEntityIdentifier = conceptsApiResponse.LeiCode
	org.YearFounded = conceptsApiResponse.YearFounded
	org.IsDeprecated = conceptsApiResponse.IsDeprecated

	formerNames := []string{}
	m := make(map[string]bool)
	uniqLabel := []string{}
	for _, label := range conceptsApiResponse.AlternativeLabels {
		compare := func(expected string) bool {
			return strings.TrimPrefix(label.Type, ontologyPrefix) == expected
		}
		switch {
		case compare("/properName"):
			org.ProperName = label.Value
		case compare("/shortName"):
			org.ShortName = label.Value
		case compare("/hiddenLabel"):
			org.HiddenLabel = label.Value
		case compare("/formerName"):
			formerNames = append(formerNames, label.Value)
		}

		if !m[label.Value] {
			m[label.Value] = true
			uniqLabel = append(uniqLabel, label.Value)
		}
	}
	if len(formerNames) > 0 {
		org.FormerNames = formerNames
	}
	if len(uniqLabel) > 0 {
		org.Labels = uniqLabel
	}

	var subsidiaries = []Subsidiary{}
	for _, item := range conceptsApiResponse.Related {
		c := item.Concept
		if strings.TrimPrefix(item.Predicate, ontologyPrefix) == hasParentPredicate {
			parent := &Parent{}
			parent.ID = convertID(c.ID)
			parent.APIURL = convertApiUrl(c.ApiURL, "organisations")
			parent.PrefLabel = c.PrefLabel
			parent.DirectType = c.Type
			parent.Types = mapper.FullTypeHierarchy(c.Type)
			org.Parent = parent
		}
		if strings.TrimPrefix(item.Predicate, ontologyPrefix) == isParentPredicate {
			subsidiary := Subsidiary{}
			subsidiary.ID = convertID(c.ID)
			subsidiary.APIURL = convertApiUrl(c.ApiURL, "organisations")
			subsidiary.PrefLabel = c.PrefLabel
			subsidiary.DirectType = c.Type
			subsidiary.Types = mapper.FullTypeHierarchy(c.Type)
			subsidiaries = append(subsidiaries, subsidiary)
		}
		if strings.TrimPrefix(item.Predicate, ontologyPrefix) == issuedPredicate {
			f := &FinancialInstrument{}
			f.ID = convertID(c.ID)
			f.APIURL = convertApiUrl(c.ApiURL, "things")
			f.PrefLabel = c.PrefLabel
			f.DirectType = c.Type
			f.Types = mapper.FullTypeHierarchy(c.Type)
			f.Figi = c.Figi
			org.FinancialInstrument = f
		}
	}
	if len(subsidiaries) > 0 {
		org.Subsidiaries = subsidiaries
	}

	return org, true, nil
}

func convertApiUrl(conceptsApiUrl string, desired string) string {
	return strings.Replace(conceptsApiUrl, "concepts", desired, 1)
}

func convertID(conceptsApiID string) string {
	return strings.Replace(conceptsApiID, ftThing, thingsApiUrl, 1)
}

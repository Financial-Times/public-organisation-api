# Public API for Organisation (public-organisation-api)
__Provides a public API for Organisation stored in a Neo4J graph database__

Organisations are being migrated to be served from the new [Public Concepts API](https://github.com/Financial-Times/public-concepts-api) and as such this API will eventually be deprecated. From July 2018 requests to this service will be redirected via the concepts api then transformed to match the existing contract and returned.

## Build & deployment etc:
_NB You will need to create a tagged release in order to build
* [Build and Deploy](https://upp-k8s-jenkins.in.ft.com/job/k8s-deployment/job/apps-deployment/job/public-organisations-api-auto-deploy/)


## Installation
        go get -u github.com/Financial-Times/public-organisation-api
        cd $GOPATH/src/github.com/Financial-Times/public-organisation-api
        dep ensure -v -vendor-only
        go test ./...
        go install

## Running locally

	Usage: public-organisations-api [OPTIONS]

	A public RESTful API for accessing organisations in Neo4j

	Options:
	      --app-system-code        System Code of the application (env $APP_SYSTEM_CODE) (default "public-organisation-api")
	      --port                   Port to listen on (env $APP_PORT) (default "8080")
	      --log-level              Log level to use (env $LOG_LEVEL) (default "debug")
	      --env                    environment this app is running in (default "local")
	      --cache-duration         Duration Get requests should be cached for. e.g. 2h45m would set the max-age value to '7440' seconds (env $CACHE_DURATION) (default "30s")
	      --publicConceptsApiURL   Public concepts API endpoint URL. (env $CONCEPTS_API) (default "http://localhost:8081")

## API definition
* Based on the following [google doc](https://docs.google.com/document/d/1SC4Uskl-VD78y0lg5H2Gq56VCmM4OFHofZM-OvpsOFo/edit#heading=h.qjo76xuvpj83)
* See the [api](_ft/api.yml) Swagger file for endpoints definitions

## Healthchecks
Healthchecks: [http://localhost:8080/__health](http://localhost:8080/__health)

### Logging
the application uses logrus, the logfile is initilaised in main.go.
 logging requires an env app parameter, for all enviromets  other than local logs are written to file
 when running locally logging is written to console (if you want to log locally to file you need to pass in an env parameter that is != local)
 NOTE: build-info end point is not logged as it is called every second from varnish and this information is not needed in  logs/splunk

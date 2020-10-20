package dockerhub

import (
	"encoding/json"
	"errors"
	"fmt"
	dads "github.com/LF-Engineering/da-ds"
	"strings"
	"time"
)

// Fetcher contains dockerhub datasource fetch logic
type Enricher struct {
	DSName                string // Datasource will be used as key for ES
	ElasticSearchProvider ESClientProvider
	BackendVersion        string
}

type EnricherParams struct {
	BackendVersion string
}

func NewEnricher(params EnricherParams, esClientProvider ESClientProvider) *Enricher {
	return &Enricher{
		DSName:                Dockerhub,
		ElasticSearchProvider: esClientProvider,
		BackendVersion:        params.BackendVersion,
	}
}

func (e *Enricher) EnrichItem(rawItem RepositoryRaw) error {

	enriched := RepositoryEnrich{}

	enriched.ID = fmt.Sprintf("%s-%s", rawItem.Data.Namespace, rawItem.Data.Name)
	enriched.IsEvent = 1
	enriched.IsDockerImage = 0
	enriched.IsDockerhubDockerhub = 1
	enriched.Description = rawItem.Data.Description
	enriched.DescriptionAnalyzed = rawItem.Data.Description

	// todo: in python description is used ??
	enriched.FullDescriptionAnalyzed = rawItem.Data.FullDescription
	enriched.Project = rawItem.Data.Name
	enriched.Affiliation = rawItem.Data.Affiliation
	enriched.IsPrivate = rawItem.Data.IsPrivate
	enriched.IsAutomated = rawItem.Data.IsAutomated
	enriched.PullCount = rawItem.Data.PullCount
	enriched.RepositoryType = rawItem.Data.RepositoryType
	enriched.User = rawItem.Data.User
	enriched.Status = rawItem.Data.Status
	enriched.StarCount = rawItem.Data.StarCount

	enriched.BackendName = fmt.Sprintf("%sEnrich", strings.Title(e.DSName))
	enriched.BackendVersion = e.BackendVersion
	timestamp := time.Now()
	enriched.MetadataEnrichedOn = dads.ToESDate(timestamp)

	enriched.MetadataTimestamp = rawItem.MetadataTimestamp
	enriched.MetadataUpdatedOn = rawItem.MetadataUpdatedOn
	enriched.CreationDate = rawItem.MetadataUpdatedOn

	// todo:
	enriched.RepositoryLabels = nil
	enriched.MetadataFilterRaw = nil
	enriched.Offset = nil


	enriched.Origin = rawItem.Origin
	enriched.Tag = rawItem.Origin
	enriched.UUID = rawItem.UUID

	body, err := json.Marshal(enriched)
	if err != nil {
		return errors.New("unable to convert body to json")
	}

	index := fmt.Sprintf("sds-%s-%s-dockerhub", rawItem.Data.Namespace, rawItem.Data.Name)

	_, err = e.ElasticSearchProvider.Add(index, enriched.UUID, body)
	if err != nil {
		return err
	}

	return nil
}

func (e *Enricher) HandleMapping(owner string, repository string) error {
	index := fmt.Sprintf(IndexPattern, owner, repository)

	_, err := e.ElasticSearchProvider.CreateIndex(index, DockerhubRichMapping)
	return err
}

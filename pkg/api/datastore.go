package api

import (
	"context"
	"time"
)

type Datastore struct {
	gqlClient Doer
}

// Doer interface is satisfied by the GraphqlQLClient.
type Doer interface {
	Do(ctx context.Context, request GQLRequest, response interface{}) error
}

func NewDatastore(gqlClient Doer) *Datastore {
	return &Datastore{
		gqlClient: gqlClient,
	}
}

type listRegionResponseData struct {
	Regions []Region `json:"Regions"`
}
type listRegionResponse struct {
	Data listRegionResponseData `json:"data"`
}

// ListRegions lists all the available runtimes.
func (ds *Datastore) ListRegions(ctx context.Context) ([]Region, error) {
	const query = `
query MyQuery {
  Regions(where: {enabled: {_eq: true}}) {
 	name    
	alias
	deployment_api_url
	assets_api_url
	debugger_url_scheme
	host_template
  }
}`
	req := GQLRequest{
		Query: query,
	}
	var resp listRegionResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Regions, nil
}

func (ds *Datastore) GetRegion(ctx context.Context, alias string) (Region, error) {
	const query = `
query MyQuery ($alias: String!) {
  Regions(where: {enabled: {_eq: true}, _and: {alias: {_eq: $alias}}}) {
    name
    alias
	deployment_api_url
	marketplace_api_url
	assets_api_url
	endpoint_url_scheme
	debugger_url_scheme
	host_template
  }
}`

	req := GQLRequest{
		Query:     query,
		Variables: map[string]string{"alias": alias},
	}
	var resp listRegionResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return Region{}, err
	}
	if len(resp.Data.Regions) == 0 {
		return Region{}, ErrNotFound
	}
	return resp.Data.Regions[0], nil
}

type getByProjAndInstNameData struct {
	Instances []Instance `json:"Instances"`
}

type getByProjAndInstNameResponse struct {
	Data getByProjAndInstNameData `json:"data"`
}

// GetByProjectAndInstanceName gets the instance by project and instance name.
func (ds *Datastore) GetInstanceByProjectAndInstanceName(ctx context.Context, projectName, instanceName string) (Instance, error) {
	const query = `
query MyQuery ($project_name: String!, $instance_name: String!) {
  Instances(where: {Project: {name: {_eq: $project_name}}, _and: {name: {_eq: $instance_name}}}) {
    id
    service_name
  }
}`
	req := GQLRequest{
		Query: query,
		Variables: map[string]string{
			"project_name":  projectName,
			"instance_name": instanceName,
		},
	}
	var resp getByProjAndInstNameResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return Instance{}, err
	}
	if len(resp.Data.Instances) == 0 {
		return Instance{}, ErrNotFound
	}
	return resp.Data.Instances[0], nil
}

type getInstanceByIDData struct {
	InstancesByPk *Instance `json:"Instances_by_pk"`
}

type getInstanceByIDResponse struct {
	Data getInstanceByIDData `json:"data"`
}

// GetInstanceByID gets the instance by ID.
func (ds *Datastore) GetInstanceByID(ctx context.Context, id string) (Instance, error) {
	const query = `
query myQuery ($id: uuid!) {
  Instances_by_pk(id: $id) {
    id
    service_name
  }
}`
	req := GQLRequest{
		Query: query,
		Variables: map[string]string{
			"id": id,
		},
	}
	var resp getInstanceByIDResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return Instance{}, err
	}
	if resp.Data.InstancesByPk == nil {
		return Instance{}, ErrNotFound
	}
	return *resp.Data.InstancesByPk, nil
}

type getRuntimeByNameParams struct {
	Name string `json:"name"`
}

type getRuntimeByNameResponseData struct {
	Runtimes []Runtime `json:"Runtimes"`
}

type getRuntimeByNameResponse struct {
	Data getRuntimeByNameResponseData `json:"data"`
}

// GetByName gets the runtime by name.
func (ds *Datastore) GetRuntimeByName(ctx context.Context, name string) (Runtime, error) {
	const query = `
query MyQuery ($name: String!) {
  Runtimes(where: {name: {_eq: $name} _and: { enabled: {_eq: true} }}) {
    id
    name
    language
	api_version
	comments
  }
}`
	req := GQLRequest{
		Query:     query,
		Variables: getRuntimeByNameParams{Name: name},
	}
	var resp getRuntimeByNameResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return Runtime{}, err
	}
	if len(resp.Data.Runtimes) == 0 {
		return Runtime{}, ErrNotFound
	}
	return resp.Data.Runtimes[0], nil
}

type listRuntimeResponseData struct {
	Runtimes []Runtime `json:"Runtimes"`
}
type listRuntimeResponse struct {
	Data listRuntimeResponseData `json:"data"`
}

// List lists all the available runtimes.
func (ds *Datastore) ListRuntimes(ctx context.Context) ([]Runtime, error) {
	const query = `
query MyQuery {
  Runtimes(where: {enabled: {_eq: true}}) {
    id
    name
    language
	comments
  }
}`

	req := GQLRequest{
		Query: query,
	}
	var resp listRuntimeResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Runtimes, nil
}

type getParams struct {
	APIAccountID string `json:"api_account_id"`
	Name         string `json:"name"`
}

type getProjectResponseData struct {
	Projects []Project `json:"Projects"`
}

type getProjectResponse struct {
	Data getProjectResponseData `json:"data"`
}

func (ds *Datastore) GetProject(ctx context.Context, accountID, name string) (Project, error) {
	const query = `
query MyQuery ($api_account_id: String!, $name: String!) {
  Projects(where: {api_account_id: {_eq: $api_account_id}, _and: {name: {_eq: $name}, _and: {deleted: {_eq: false}}}}) {
    id
    name
    api_account_id
    created_at
    updated_at
  }
}`
	req := GQLRequest{
		Query:     query,
		Variables: getParams{APIAccountID: accountID, Name: name},
	}
	var resp getProjectResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return Project{}, err
	}
	if len(resp.Data.Projects) == 0 {
		return Project{}, ErrNotFound
	}
	return resp.Data.Projects[0], nil
}

type listProductResponseData struct {
	Products []Product `json:"Products"`
}
type listProductResponse struct {
	Data listProductResponseData `json:"data"`
}

func (ds *Datastore) ListProducts(ctx context.Context) ([]Product, error) {
	const query = `
query MyQuery {
  Products(where: {ProductVersions: {code_template_enabled: {_eq: true}}, type: {_eq: public}}, order_by: {name: asc}) {
    id
    name
    programming_language
  }
}`

	req := GQLRequest{
		Query: query,
	}
	var resp listProductResponse

	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Products, nil
}

type getLatestProductVersionByIDParams struct {
	ID string `json:"id"`
}

type getLatestProductVersionByIDResponseData struct {
	ProductVersions []ProductVersion `json:"ProductVersions"`
}

type getLatestProductVersionByIDResponse struct {
	Data getLatestProductVersionByIDResponseData `json:"data"`
}

func (ds *Datastore) GetLatestProductVersionByID(ctx context.Context, id string) (ProductVersion, error) {
	const query = `
query MyQuery ($id: uuid!) {
  ProductVersions(where: {Product: {id: {_eq: $id}}}, order_by: {created_at: desc}) {
    id
  }
}
`
	req := GQLRequest{
		Query:     query,
		Variables: getLatestProductVersionByIDParams{ID: id},
	}
	var resp getLatestProductVersionByIDResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return ProductVersion{}, err
	}
	if len(resp.Data.ProductVersions) == 0 {
		return ProductVersion{}, ErrNotFound
	}
	return resp.Data.ProductVersions[0], nil
}

type getLogByIDParams struct {
	ID        string    `json:"instance_id"`
	Limit     int       `json:"limit"`
	Timestamp time.Time `json:"timestamp"`
}

type listLogResponseData struct {
	Logs []Log `json:"Logs"`
}
type listLogResponse struct {
	Data listLogResponseData `json:"data"`
}

// ListLogs lists all the available logs.
func (ds *Datastore) ListLogsByInstanceID(ctx context.Context, id string, limit int, timestamp time.Time) ([]Log, error) {
	const query = `
query MyQuery ($instance_id: String!, $limit: Int!, $timestamp: Time!) {
  Logs(where: {instance_id: {_eq: $instance_id}, timestamp: {_gt: $timestamp}}, order_by: {timestamp: desc}, limit: $limit) {
    message
    timestamp
  }
}`
	req := GQLRequest{
		Query:     query,
		Variables: getLogByIDParams{ID: id, Limit: limit, Timestamp: timestamp},
	}
	var resp listLogResponse
	if err := ds.gqlClient.Do(ctx, req, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Logs, nil
}

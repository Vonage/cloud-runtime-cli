CREATE TABLE IF NOT EXISTS "Projects" (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255),
    api_account_id VARCHAR(255),
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted BOOLEAN
    );

INSERT INTO "Projects" (id, name, api_account_id, created_at, updated_at, deleted) VALUES ('test-id', 'project1', 'test_key', '2021-01-01 00:00:00', '2021-01-02 00:00:00', false);

CREATE TABLE IF NOT EXISTS "Regions" (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255),
    alias VARCHAR(255),
    enabled BOOLEAN,
    deployment_api_url VARCHAR(255),
    assets_api_url VARCHAR(255),
    endpoint_url_scheme VARCHAR(255),
    debugger_url_scheme VARCHAR(255),
    host_template VARCHAR(255)
    );

INSERT INTO "Regions" (name, alias, enabled, deployment_api_url, assets_api_url, endpoint_url_scheme, debugger_url_scheme, host_template) VALUES ('Region One', 'aws.euw1', true, 'http://mockserver:80', 'https://assets.region1.example.com', 'https', 'https', 'region1.example.com');

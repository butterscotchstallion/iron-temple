## About

This is a fitness tracking web app intended to track weight lifting sessions over time.

- No authentication required at this time

## Back end

- api folder
- Go
- https://github.com/go-chi/chi for the REST API
- OpenAPI v3 spec which will be used for code generation on the front end
- PostgreSQL for database storage


## Front end

- ui folder
- Typescript
- Svelte
- Tailwind
- The API services should be generated using https://heyapi.dev - npm install @hey-api/openapi-ts -D -E
- Mobile first - should look good on an iPad

## Pre-commit tooling

- Go
  - govet
  - go mod tidy
  - golangci-lint
  - run tests
- Front end
  - validate typescript
  - run heyapi to generate API services
  - run tests

## How the app works

- Programs
  - Each program is made up of exercises
  - Each exercise has an associated weight and number of repetitions
  - The available programs will be based on the programs listed on https://stronglifts.com
- Sessions
  - Each session consists of a program day's exercises
  - Typically a program has a series of variations based on the day with exercises assigned to that day
  - Each set of repetitions for an exercise is separated by a 3 minute break

## Implementation plan

### Phase 1 - Database initialization and design

- Create tables and relationships based on above description
- Enforce data integrity
- Use best practices

### Phase 2 - Implement OpenAPI v3 specification

- Write integration tests using httpexpect that are red first
- Then fix tests by implementing API routes


### Phase 3 - Implement REST API according to OpenAPI specification

- Implement REST API in the api folder
- Configure API to allow CORS from the front end

### Phase 4 - Build front end

- Going for a Synthwave vibe with strong purple elements
- Initialize front end in the ui folder
- Use the technologies listed above
- Add unit tests and Playwright tests












# Description
This package integrate features of postgres and mongo databases. Features include CRUD and transaction.
~~This package also support load balancing which can connect to database cluster.~~

## Concerns
- Handle large volume size of data
  - for query, create the cache in front of the database server? what the TTL for each cache? 
  - build database replicas(master and slave), separate read and write operations
- collaborate with cache service (not implement)
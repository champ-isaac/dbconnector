# Description
This package integrate features of postgres and mongo databases. Features include CRUD and transaction.
~~This package also support load balancing which can connect to database cluster.~~

## Note
Please check examples in `/tests` to see how to use dbconnector.

## Architecture
![dbconnector architecture](Privacy%20API%20Service%20Diagram.png)

## Concerns
- Handle large volume size of data write into database
  - for query, create the cache in front of the database server? what the TTL for each cache? 
  - build database replicas(master and slave), separate read and write operations
- collaborate with cache service (not implement)
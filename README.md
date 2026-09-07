# SimpleLoadBalancer

Load Balancer implemented in go to balance traffic between instances of a server maintained using docker containers

## How to run

Just run the command

```bash
docker compose up --scale backend=3 -d
```

Then access the loadbalancer with the url `http://localhost:8080`

## How to test

Run the Go unit tests for the load balancer:

```bash
cd loadbalancer
go test ./...
```

For a simple stress test, install [hey](https://github.com/rakyll/hey) and send concurrent traffic to the load balancer:

```bash
hey -n 10000 -c 50 http://localhost:8080/
```

You can also run the pressure test from Go once `hey` is installed locally:

```bash
cd loadbalancer
go test -run TestLoadBalancerUnderPressureWithHey -v

OR 

go test -v  // to see all test results
```

If you want a backend to occasionally fail health checks while you test recovery, set `HEALTH_FAIL_RATE` on the server container to a value from 1 to 100.

## Testing Service Discovery
You can also manually test the service discovery feature of the Loadbalancer by running a separate docker container that is in the network of the LB, as long as it's hostname (the way the LB detects service) is backend, it gets added to the list of servers available

```bash
cd server

docker build . -t <image-name>
docker run -e SERVER_NAME=outside-server -e SERVER_TYPE=external_backend -h backend --network simpleloadbalancer_my-network -p 8090:8080 <image-name>

```
## Inspiration

I learned most of how it works from looking online, though my main source was this [https://www.researchgate.net/publication/388612894_A_Comprehensive_Study_of_Load_Balancing_Architectures_in_Cloud_Computing](article)

And also a tiny bit from [RFC 8482](https://datatracker.ietf.org/doc/html/rfc8482) I had difficulty understanding what I was reading, so not much was implemented from the article, but nonetheless I'll mention it.

## How I tested it

I used [https://github.com/rakyll/hey](Hey) to stress test the application, using various parameters to see how it handled traffic

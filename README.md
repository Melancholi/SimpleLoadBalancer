# SimpleLoadBalancer

<img src="./SimpleLoadBalancer.png" alt="SimpleLoadBalancer architecture" width="200" height="100">

Load Balancer implemented in go to balance traffic between instances of a server maintained using docker containers

## How to run

Just run the command, use make

```bash
make build 

#x is the quantity of servers you want running
make run COUNT=x 
```

Then access the loadbalancer with the url `http://localhost:8080`

## Admin panel

An admin panel shows the load balancer metrics: total requests, requests per second, backend health, and requests/errors per backend.

The panel is protected with HTTP Basic Auth. Set your own credentials in a `.env` file first (it is git-ignored). Compose refuses to start without it:

```bash
cp .env.example .env
# then edit ADMIN_USERNAME and ADMIN_PASSWORD
```

After `docker compose up`, open `http://localhost:8081` and log in with those credentials.

The load balancer serves the raw metrics as JSON on port `9090` (`/metrics`). That port is only reachable inside the docker network, so the admin panel is the only way to see the metrics from the host.

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

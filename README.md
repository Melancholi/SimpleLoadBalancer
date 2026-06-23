# SimpleLoadBalancer

Load Balancer implemented in go to balance traffic between instances of a server maintained using docker containers

## How to run

Just run the command

```bash
docker compose up -d
```

Then access the loadbalancer with the url `http://localhost:8080`

## Inspiration

I learned most of how it works from looking online, though my main source was this [https://www.researchgate.net/publication/388612894_A_Comprehensive_Study_of_Load_Balancing_Architectures_in_Cloud_Computing](article)

## How I tested it

I used [https://github.com/rakyll/hey](Hey) to stress test the application, using various parameters to see how it handled traffic

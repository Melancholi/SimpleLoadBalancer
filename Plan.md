# Load Balancer

## Characteristics:

### Version 1

- Have a server running on X Ip address at X port

Upon connection from a user (Only a get request for now) trigger event:

- From list of discovered servers, through X algorithm send user to that IP
- Algorithm can be through round robin or other common load balancer strategies

### Version 2:

Middleware that can be added to any web app

- Feature: Create configs that allow the loadbalancer to manage a list of servers given by the user
- Req: Find library for configs given on run time or through files

- Feature: Health checks to make sure server is alive before sending to user
- Req: Look up goroutines/processes that run 24/7 in the background while main process is running

## Technologies to use:

- Sockets **(Do I need to use them after all?)**
- Goroutines (to handle multiple requests at the same time) **(Look into Goroutines)**


## Thought Process

### Attaching to any website to use

My main issue at the moment is figuring out how I'll attach the load balancer to any website that I want to use the service with.

The requirements:

Loadbalancer instance running somewhere

Multiples instances of server to distribute traffic with

#### Approach 1: Dockerization
**Pros**: So, one approach I already did was with Dockerization, have all the services be in the same network and compose makes it easy to setup

**Cons**: Requires manually managing and setting up the resources for the servers, which seems tedious

#### Approach 2: Hardware
Have a machine run multiple instances of the servers, while having a loadbalancer redistribute everything 

Pros: Easier setup, less configuration troubles
Cons: requires in house hardware, single point of failure if hardware fails

#### Approach 3: Add-on

Possibly make is to that the LB is running on it's own instance, and have other resources connect to it when need be with a register/unregister endpoint

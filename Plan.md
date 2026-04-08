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
- 
# taiyoukei
Planetary system with gRPC. [Please have a look at my blog for more on this project.](https://medium.com/p/32ee1bb12e45/edit)


From the root, in that order
- start the central server with 
```
go run server/run.go
```
- start the capture client to generate the orbit graphs
```
go run capture/main.go
```
- start the S client for the Sun
```
go run client/run.go -name S -mass 1 -x -0.0000030393 -y 0 -vx 0 -vy 0
```
- start the E client for the Earth
```
go run client/run.go -name E -mass 0.0000030025 -x 0.999997 -y 0 -vx 0 -vy 1
```
- start the M client for the Moon
```
go run client/run.go -name M -mass 0.000000036938 -x 0.997427 -y 0 -vx 0 -vy 0.965816
```

The graphical outputs from the capture client are generated on a regular basis and when SIGTERM'd. 

To stop the processes, first terminate the capture, then you can simply terminate the server, it will automatically terminate the clients.

Expected plotting outputs should be like:
- for the SME system
![celestial_positions](https://github.com/Gilt0/taiyoukei/assets/82427864/0c246f7d-5149-4cf7-868a-c58af28d1d3f)
- for the ME system
![relative_positions_M_to_E](https://github.com/Gilt0/taiyoukei/assets/82427864/1f6771ff-80cd-4684-b781-5946e0e9a42f)

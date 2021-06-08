# Bridge-Synchronization
A classic distributed system problem of shared resource synchronization

Scenario and Updated Mutual Exclusion Algorithm
=================================================
There are 4 Cars of two different colors. Two of them are blue, denoted by B1 and B2, rest two are red in color, denoted by R1 and R2. All these cars actually represent different node in a distributed system. And they need to cross a single lane bridge (represents the Critical Section) which connects two different cities. We have to resolve the problem by implementing, applying and updating Mutual exclusion any algorithm.

We have to update the any of the Mutual Exclusion Algorithm in such a way that it can allow multiple Critical Section (CS) access on certain condition. In the given problem, the condition is the direction of Car’s movement on the Bridge. We choose Lamport’s Algorithm to update to satisfy the requirement. Our updated algorithm steps are given below.

1. Use Lamport’s Algorithm to determine initial access to the bridge (CS).
2. As soon as the Car get the Bridge access, it will let others know that it is on the bridge and it’s moving direction by sending SetBridgeState Message. Other cars will update the bridge owner and direction by getting the message
3. If any other car appears to the bridge and found the any other car is crossing the bridge in the it’s desired direction, it will not send REQUEST message to others, it will send SetBridgeState message to other Cars and access the Bridge.
4. While any car gets off the bridge, it will let other know by sending ResetBridgeState message. If the message is received from the current owner of the bridge, the bridge direction and owner will be reset.
5. If any car appears the bridge and the traffic of the bridge is moving the opposite of its desired direction, then it will send REQUEST message to access the bridge, which will be considered after the direction of the bridge will be reset.

The RPC Server will display the Track map, containing two cities connected by a single lane bridge and the current position and movement of all the cars, and it will control the execution duration for all the car processes.

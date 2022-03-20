# Raft Consensus Algorithm

<div id="top"></div>
<!--
*** Thanks for checking out the Best-README-Template. If you have a suggestion
*** that would make this better, please fork the repo and create a pull request
*** or simply open an issue with the tag "enhancement".
*** Don't forget to give the project a star!
*** Thanks again! Now go create something AMAZING! :D
-->

<!-- PROJECT SHIELDS -->
<!--
*** I'm using markdown "reference style" links for readability.
*** Reference links are enclosed in brackets [ ] instead of parentheses ( ).
*** See the bottom of this document for the declaration of the reference variables
*** for contributors-url, forks-url, etc. This is an optional, concise syntax you may use.
*** https://www.markdownguide.org/basic-syntax/#reference-style-links
-->

[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![MIT License][license-shield]][license-url]
[![LinkedIn][linkedin-shield]][linkedin-url]

<!-- PROJECT LOGO -->
<br />
<div align="center">
  <a href="https://github.com/othneildrew/Best-README-Template">
    <img src="images/logo.png" alt="Logo" width="80" height="80">
  </a>

  <h3 align="center">Best-README-Template</h3>

  <p align="center">
    An awesome README template to jumpstart your projects!
    <br />
    <a href="https://github.com/othneildrew/Best-README-Template"><strong>Explore the docs »</strong></a>
    <br />
    <br />
    <a href="https://github.com/othneildrew/Best-README-Template">View Demo</a>
    ·
    <a href="https://github.com/othneildrew/Best-README-Template/issues">Report Bug</a>
    ·
    <a href="https://github.com/othneildrew/Best-README-Template/issues">Request Feature</a>
  </p>
</div>

<!-- TABLE OF CONTENTS -->
<details>
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#about-the-project">About The Project</a>
      <ul>
        <li><a href="#built-with">Built With</a></li>
      </ul>
    </li>
    <li>
      <a href="#getting-started">Getting Started</a>
      <ul>
        <li><a href="#prerequisites">Prerequisites</a></li>
        <li><a href="#installation">Installation</a></li>
      </ul>
    </li>
    <li><a href="#project-details">Project Details</a></li>
    <li><a href="#usage">Usage</a></li>
    <li><a href="#license">License</a></li>
    <li><a href="#contact">Contact</a></li>
    <li><a href="#acknowledgments">Acknowledgments</a></li>
  </ol>
</details>

<!-- ABOUT THE PROJECT -->

## About The Project

![Screen Shot](images/overall.png)

This project demonstrates the implementation of the `Raft Consensus algorithm` which is a consensus bases protocol for distributed systems. This project is built as a part of the course `CS60002` **_Distributed Systems_** at Indian Institute of Technology, Kharagpur. This project implements a simple version of the raft protocol, which can be used as a base template to build your own distributed system by adding features. Following are the core features implemented in this projects:

- Raft Consensus RPCs
  - `RequestVote` RPC
  - `AppendEntries` RPC
- Raft Log
  - `Log` class
- Raft State Machine
  - `StateMachine` (a simple state machine)
- Raft Leader Election
  - `LeaderElection` RPC
- Raft Consensus
  - `RaftConsensus` class
- `Membership Change` Feature
- Visualization with `timing diagram`
- Single `client interface` for testing the features

A single client interface was built mainly because this is a simple working protoype and not industrial-strength distributed system. The client interface is a simple command line interface which can be used to test the features of the project. All the RPCs are implemented in accordance with [In Search of an Understandable Consensus Algorithm](https://raft.github.io/raft.pdf) by Diego Ongaro and John Ousterhout. This implementation of the raft can be used as a base model and can be extended to build your own distributed system by adding advanced features and implementing multiple client gateway.

<p align="right">(<a href="#top">back to top</a>)</p>

### Built With

Following mentioned are the major frameworks/libraries used to bootstrap this project. Leave any add-ons/plugins for the acknowledgements section. Here are a few examples.

- [Golang](https://go.dev/)
  - [Leak Test](github.com/fortytw2/leaktest) - _Required for memory leak testing_
  - [Pretty Tables](github.com/jedib0t/go-pretty) - _Used in Timing Diagram visualization_
  - [net/rpc](https://pkg.go.dev/net/rpc) - _Wrappers required for building Raft RPCs_
- [Shell Script](https://www.javatpoint.com/shell-scripting-tutorial)

<p align="right">(<a href="#top">back to top</a>)</p>

## Project Details

Add the details of the project here. Mainly add the following

- [ ] Description of each file and their functioning
- [ ] If the code is divided in classes or as per use case, mention the working of the class
- [ ] Mention the important variables and describe them
- [ ] If possible add a short theory to support your descriptions

```
raft-consensus
├──── LICENSE
├──── README.md
├──── go.mod
├──── go.sum
├──── images
│     └── overall.png
├──── main.go
├──── raft
│     ├── config.go
│     ├── raft.go
│     ├── raft_test.go
│     ├── server.go
│     ├── simulator.go
│     └── storage.go
└──── utils
      ├── logs
      ├── visualize.sh
      ├── viz.go
      └── viz.txt
```

Following are the details of the file structure and their functionalities that are present in this code base.

- **raft/server.go** - _This file contains all the necessary code for implementing servers in a network using TCP along with various Remote Procedural Calls_
  - `Server struct` - Structure to define a service object
  - `Server` methods - Methods to implement the server
    - **_CreateServer\:_** create a Server Instance with serverId and list of peerIds
    - **_ConnectionAccept\:_** keep listening for incoming connections and serve them
    - **_Serve\:_** start a new service
    - **_Stop\:_** stop an existing service
    - **_ConnectToPeer\:_** connect to another server or peer
    - **_DisconnectPeer\:_** disconnect from a particular peer
    - **_RPC\:_** make an RPC call to the particular peer
    - **_RequestVote\:_** RPC call from a raft node for RequestVote
    - **_AppendEntries\:_** RPC call from a raft node for AppendEntries
- **raft/simulator.go** - _This file contains all the necessary code to setup a cluster of raft nodes, interact with the cluster and execute different commands such as read, write and config change on the cluster._
  - `ClusterSimulator` struct - Structure to define a Raft cluster
  - `Simulator` methods - Methods to implement the cluster
    - **_CreateNewCluster\:_** create a new Raft cluster consisting of a given number of nodes and establish
    - connections between them
    - **_Shutdown\:_** shut down all servers in the cluster
    - **_CollectCommits\:_** reads channel and adds all received entries to the corresponding commits
    - **_DisconnectPeer\:_** disconnect a server from other servers
    - **_ReconnectPeer\:_** reconnect a disconnected server to other servers
    - **_CrashPeer\:_** crash a server and shut it down
    - **_RestartPeer\:_** restart a crashed server and reconnect to other peers
    - **_SubmitToServer\:_** submit a command to a server
    - **_Check_Functions\:_** auxiliary helper functions to check the status of the raft cluster: CheckUniqueLeader, CheckNoLeader and CheckCommitted
- **raft/raft_test.go** - _This file has a set of test functions designed to test the various functionalities of the raft protocol. The tests can be designed into 3 major classes:_
  - **_Tests to check Leader Election_**
  - **_Tests to check Command Commits_**
  - **_Tests to check Membership Changes_**
- **raft/config.go** - _This file has a custom implementation of a Set Data Structure as it is not provided inherently by Go. This implementation is inspired by [Set in Golang](https://golangbyexample.com/set-implementation-in-golang/). It provides the following functions:_
  - **_makeSet_**
  - **_Exists_**
  - **_Add_**
  - **_Remove_**
  - **_Size_**

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- GETTING STARTED -->

## Getting Started

This is an example of how you may give instructions on setting up your project locally.
To get a local copy up and running follow these simple example steps.

### Prerequisites

- **Go**  
  To run the code in this Assignment, one needs to have Go installed in their system. If it is not
  already installed, it can be done by following the steps in [Install Go Ubuntu](https://www.tecmint.com/install-go-in-ubuntu/#:~:text=To%20download%20the%20latest%20version,download%20it%20on%20the%20terminal.&text=Next%2C%20extract%20the%20tarball%20to%20%2Fusr%2Flocal%20directory.&text=Add%20the%20go%20binary%20path,a%20system%2Dwide%20installation)

### Installation

_In order to setup a local copy of the project, you can follow the one of the 2 methods listed below. Once the local copy is setup, the steps listed in [Usage](#usage) can be used to interact with the system._

1. Clone the repo
   ```sh
   git clone https://github.com/debajyotidasgupta/raft-consensus
   ```
2. Unzip the attached submission to unpack all the files included with the project.  

<p align="right">(<a href="#top">back to top</a>)</p>

### Setting DEBUG level
_In order to obtain logs regarding the execution of Raft algorithm you need to set DEBUG variable as 1 inside raft/raft.go_  
_Similarly if you do not wish to see huge logs and just see the outputs of execution you can set the DEBUG level to 0_

- [ ] Include details about git clone and running after unzipping submission
- [ ] Installing dependencies
- [ ] Set `DEBUG` and any other **_SECRET KEY_**

<!-- USAGE EXAMPLES -->

## Usage

Use this space to show useful examples of how a project can be used. Additional screenshots, code examples and demos work well in this space. You may also link to more resources.

- [ ] Write about main.go

### User interaction with the system

_To interact with the system from the console, do the following steps\:_

1.  Open terminal from the main project directory
2.  Run the main go file
    ```sh
    go run main.go
    ```
3.  You will be presented with a menu with necessary commands to create raft cluster, send commands, etc.

### Running tests 

_A comprehensive set of tests has been provided in **raft/raft_test.go**. In order to run these tests, do the following steps\:_

1.  To run a particular test execute the following command from the main project directory
    ```sh
    go test -timeout 30s -v -run ^[Test Name]$ raft-consensus/raft
    ```
2.  To run the entire test suite run the following command from the main project directory
    ```sh
    go test -v raft-consensus/raft
    ```

- [ ] Write about visualizations
- [ ] Add screenshots

_For more examples, please refer to the [Documentation](https://example.com)_

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- LICENSE -->

## License

Distributed under the MIT License. See `LICENSE.txt` for more information.

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- CONTACT -->

## Contact

Your Name - [@your_twitter](https://twitter.com/your_username) - email@example.com

Project Link: [https://github.com/your_username/repo_name](https://github.com/your_username/repo_name)

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- ACKNOWLEDGMENTS -->

## Acknowledgments

Use this space to list resources you find helpful and would like to give credit to. I've included a few of my favorites to kick things off!

- [Choose an Open Source License](https://choosealicense.com)
- [GitHub Emoji Cheat Sheet](https://www.webpagefx.com/tools/emoji-cheat-sheet)
- [Malven's Flexbox Cheatsheet](https://flexbox.malven.co/)
- [Malven's Grid Cheatsheet](https://grid.malven.co/)
- [Img Shields](https://shields.io)
- [GitHub Pages](https://pages.github.com)
- [Font Awesome](https://fontawesome.com)
- [React Icons](https://react-icons.github.io/react-icons/search)

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- MARKDOWN LINKS & IMAGES -->
<!-- https://www.markdownguide.org/basic-syntax/#reference-style-links -->

[contributors-shield]: https://img.shields.io/github/contributors/othneildrew/Best-README-Template.svg?style=for-the-badge
[contributors-url]: https://github.com/othneildrew/Best-README-Template/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/othneildrew/Best-README-Template.svg?style=for-the-badge
[forks-url]: https://github.com/othneildrew/Best-README-Template/network/members
[stars-shield]: https://img.shields.io/github/stars/othneildrew/Best-README-Template.svg?style=for-the-badge
[stars-url]: https://github.com/othneildrew/Best-README-Template/stargazers
[issues-shield]: https://img.shields.io/github/issues/othneildrew/Best-README-Template.svg?style=for-the-badge
[issues-url]: https://github.com/othneildrew/Best-README-Template/issues
[license-shield]: https://img.shields.io/github/license/othneildrew/Best-README-Template.svg?style=for-the-badge
[license-url]: https://github.com/othneildrew/Best-README-Template/blob/master/LICENSE.txt
[linkedin-shield]: https://img.shields.io/badge/-LinkedIn-black.svg?style=for-the-badge&logo=linkedin&colorB=555
[linkedin-url]: https://linkedin.com/in/othneildrew
[product-screenshot]: images/screenshot.png

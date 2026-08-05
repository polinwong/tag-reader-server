NFC Server architecture
=============================================
By Kevin Mak, document create and update at 2/12/2024

![Architecture diagram](./arch-diagram.png)

## Information of component

### 1. Main Server
Current server was deployed on AWS with Debian Linux OS. It required use SSH and private key to access, for security problem code will not include that key. Please contact IT admin to get it.

### 2. Golang server
Go is a modern and propose fast development web service program language. If program project not include any C/C++ binding it can support cross compile on any platform. For this NFC server is using native Golang for all code so it can compile with default Go toolchain.

This project have used Iris web framework for provide fast development architecture. 

This project include a fast compile script for build the deploy binary. Script is bash language please using bash to run it

- [The Go Programming Language](https://go.dev/)
- [Iris Web Framework](https://github.com/kataras/iris)

### 3. WebUI (User and Admin)
Some of technical stage and development efficiency problem, the User page and admin page used different architecture. User page is Vue 3 in UMD with Quasar framework. Admin page is based Go iris template with Bootstrap@4.5.3 and jquery@3.5.1 

User page source path: `local/html/verifysrc`, `local/js/verify`

- [Vue.js](https://vuejs.org/)
- [Quasar framework](https://quasar.dev/)

Admin page source path: `local/html`, `local/js/admin`

- [Bootstrap 4](https://getbootstrap.com/docs/4.6/getting-started/introduction/)

### 4. bbolt key-value Database
The NFC server was not have much linked information so we used Key-Value database for manage the NFC tag and Art information.

The project used "bbolt" as database which advantage for native support Go with out using SQL must linking to C/C++ library. It can decrease the code build complicated for pre-built library linking and using go internal package manager for simple setup.

- [bbolt](https://github.com/etcd-io/bbolt)

### 5. Accessing (User and Admin)
User and admin are using HTTP(S) to access the server, which is use server API to access and edit all information. Therefor also provide smart phone Apps access to seal NFC tag.

- [NFC Apps source (Only in company local network)](http://192.168.16.13:2080/kevin.mak/tag-reader-app)

### 6. Other

#### The `climgt` project
Because of a lot of project was built by Go language that we have made a package for build the CLI interface. Please init and sync Git submodule before start to build this project.

#### The `ancientAuth` source (Deprecated)
This project have deprecated because project have no deep development. But program still reserved API access please notice that.
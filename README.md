A minimalistic TUI-based email composer, without the pretense of offering the same functionalities of a full email client.
It can only help in creating and sending emails, and was created with the following use cases in mind:
- sysadmins that want to quickly test smtp gateways
- quickly composing and sending a short email without leaving the command line (i.e. sending yourself or somebody else a short note or a link)

### Installation
Ensure you have installed golang and make:
```
sudo apt install golang  # Ubuntu
sudo apt install make
```
Clone the repo, and run:
```
make install
mailo
```

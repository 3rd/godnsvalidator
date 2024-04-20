**godnsvalidator** is a simple DNS server validator that replaces [dnsvalidator](https://github.com/vortexau/dnsvalidator).

Install:

```sh
go install -v github.com/3rd/godnsvalidator
```

Use:

```sh
wget https://github.com/trickest/resolvers/blob/main/resolvers-community.txt -O input.txt
godnsvalidator -servers input.txt -root example.com -timeout 1 -threads 20 -o resolvers.txt
```

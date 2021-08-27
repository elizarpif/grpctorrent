# Torrent
## Description of the task
You need to develop peer-to-peer file loader realizes the next requirements:

- Information about the file to upload stored in a defined handler: it stores information about division of the file into samll fragments
- Each client reports data about small fragments to the central server. The client have already downloaded all fragments and it suggests to take them to other clients
- The client contacts to server for get a list of the available clients who are owners of the interest file
- The client contacts to other clients by each fragment
- After collecting all the pieces, the client collects the file and reports that the file has been downloaded and is available

## Realization 

### api 
API description at the grpc paradigm 

### tracker
The central server. It stores information about peers 

### peer
The "torrent"-client 
## Work example | Пример работы

- launch the server 
```shell script
cd tracker
go build .
./tracker
```
-  launch the first client (it will upload the file) 
```shell script
cd peer
go build .
./peer -http=8002 -grpc=9002
```

- launch the second client (it will upload file from the first client) 
```shell script
cd peer
go build .
./peer -http=8000 -grpc=9000
```

- upload file to the server 
```shell script
curl -d "{\"name\":\"/home/space/5 sem/networks/grpctorrent/peer/some.txt\"}" -X POST http://localhost:8002/upload | jq
```

- ask information about the file 
```shell script
curl http://localhost:8002/files/some.txt | jq
```
```json
{
  "name": "some.txt",
  "piece_length": "1",
  "pieces": "24",
  "length": "24",
  "hash": "9702842ac5824617babda6a32791ac2f"
}
```

- download the file 
```shell script
curl -d "{\"hash\":\"9702842ac5824617babda6a32791ac2f\"}" -X POST http://localhost:8000/download | jq
```
```json
{
  "name": "some.txt",
  "piece_length": "1",
  "pieces": "24",
  "length": "24",
  "hash": "9702842ac5824617babda6a32791ac2f"
}
```

- get the list of the all files on the tracker-server 
```shell script
curl http://localhost:8000/files | jq
```
```json
{
  "count": "1",
  "files": [
    {
      "name": "some.txt",
      "piece_length": "1",
      "pieces": "24",
      "length": "24",
      "hash": "9702842ac5824617babda6a32791ac2f"
    }
  ]
}

```

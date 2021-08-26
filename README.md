# Torrent
## Description of the task | Задание
You need to develop peer-to-peer file loader realizes the next requirements:

- Information about the file to upload stored in a defined handler: it stores information about division of the file into samll fragments
- Each client reports data about small fragments to the central server. The client have already downloaded all fragments and it suggests to take them to other clients
- The client contacts to server for get a list of the available clients who are owners of the interest file
- The client contacts to other clients by each fragment
- After collecting all the pieces, the client collects the file and reports that the file has been downloaded and is available

|

Необходимо разработать peer-to-peer файловый «загрузчик», реализующий следующие требования:

- Информация о файле для загрузки хранится в определённом хандлере: в нём хранится информация о разбиении файла на небольшие фрагменты.
- Каждый клиент сообщает на централизованный сервер информацию о всех кусочках, которые клиент уже скачал и предлагает взять другим клиентам.
- Клиент обращается к серверу для списка доступных клиентов, которые владеют интересующемся файлом.
- Клиент обращается к другим клиентам по каждому из кусочков.
- Собрав все куски клиент собирает файл и сообщает, что файл скачан и доступен.

## Realization | Реализация

### api 
API description at the grpc paradigm | Описание апи в парадигме grpc

### tracker
The central server. It stores information about peers | Централизованный сервер. Хранит информацию о пирах

### peer
The "torrent"-client | "Торрент-клиент" 


## Work example | Пример работы

- launch the server | запускаем сервер
```shell script
cd tracker
go build .
./tracker
```
-  launch the first client (it will upload the file) | запускаем 1 клиента (тот, который загрузит файл)
```shell script
cd peer
go build .
./peer -http=8002 -grpc=9002
```

- launch the second client (it will upload file from the first client) | запускам 2 клиента (тот, который скачает файл у 1 клиента)
```shell script
cd peer
go build .
./peer -http=8000 -grpc=9000
```

- upload file to the server | загружаем файл на сервер
```shell script
curl -d "{\"name\":\"/home/space/5 sem/networks/grpctorrent/peer/some.txt\"}" -X POST http://localhost:8002/upload | jq
```

- ask information about the file | спрашиваем информацию о файле на стороне клиента
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

- download the file | скачиваем файл
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

- get the list of the all files on the tracker-server | список всех файлов на трекер-сервере
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

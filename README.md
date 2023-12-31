# Project test full stack GO + Next

- Golang backend
- Next.js + Typescript frontend
- Postgres DB
- Docker


## Docker
### To check the content of the database:

```
docker exec -it db psql -U postgres
select * from users;
```

### Build the image and run the container
```
docker compose build
docker compose up -d nextapp
```

### Check if containers are running
```
docker ps -a
```
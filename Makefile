.PHONY: slackarchive-image

slackarchive-image: maybe-build-frontend
	docker build -t slackarchive .

slackarchive-bin: maybe-build-frontend libc-wrappers.a(libc-wrappers.o)
	go build -trimpath  -ldflags "-extldflags '-Wl,--wrap,pthread_sigmask libc-wrappers.a' -linkmode external" -o ./slackarchive ./main.go

maybe-build-frontend:
	if ! [ -d "./frontend/dist" ]; then (cd frontend/ ; make); fi

libc-wrappers.a(libc-wrappers.o): libc-wrappers.o

clean:
	(cd frontend/ ; make clean)
	rm ./slackarchive

.PHONY: slackarchive

slackarchive: libc-wrappers.a(libc-wrappers.o)
	go build -trimpath  -ldflags "-extldflags '-Wl,--wrap,pthread_sigmask libc-wrappers.a' -linkmode external" -o ./slackarchive ./main.go

libc-wrappers.a(libc-wrappers.o): libc-wrappers.o



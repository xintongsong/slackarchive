.PHONY: frontend

frontend:
	docker build -t slackarchive-frontend-builder .
	docker run -v $(shell pwd):/frontend slackarchive-frontend-builder

clean:
	rm -rf node_modules dist

# SlackArchive

This project is a variant of [ashb/slackarchive](https://github.com/ashb/slackarchive) and [dutchcoders/slackarchive](https://github.com/dutchcoders/slackarchive).
It has also changed quite a lot compared to the upstream projects, in order to better work with the up-to-date Slack APIs.

## Prerequisites

- Ensure both Docker and Docker Compose are installed
- [Create an app](https://api.slack.com/) in Slack
    - You would need the following permissions: `channels:history`, `channels:join`, `channels:read`, `files:read`, `links:read`, `metadata.message:read`, `reactions:read`, `team:read`, `users:read`
    - Install your app to your workspace, and you should get an OAuth Token (starting with `xoxb-`)

## Configuration

- Create a configuration file: `cp config.yaml.sample config.yaml`.
- Edit the configuration file and replace everything wrapped with `<>`. 
    - `<xoxb-token>` - The OAuth Token you get when installing your app to your workspace.
    - `<team-domain>` - The unique domain of your Slack workspace. E.g., for `my-domain.slack.com`, `<team-domain>` should be `my-domain`.
    - `<randome-token-x>` - Random token. You can generate a random token with `ping -c 1 yahoo.com |md5 | head -c24; echo`. 
- Edit `docker-compose.yaml`, replace `<local-backup-dir>` with a local path. This is where the database dumps will be created.

## Build the Images

```
make
```

## Prepare the Database

If you are setting up a new archive, you need an empty database with proper schemes.
- Edit `docker-compose.yaml`, set `services.slackarchive.command` to `[init]`.
- Run `docker-compose up`.

If you are recovering from a previous dump of database.
- Put the dump file in `<local-backup-dir>` (see [Configuration](#configuration)).
- Edit `docker-compose.yaml`, remove `services.slackarchive` and everything in it.
- Run `docker-compose up`.
- Login to the database container: `docker exec -it $(docker ps -aqf'name=slackarchive_postgres') /bin/bash`.
- Restore from the dump file: `psql -U postgres -f backups/<your-dump-file>`.

## Start the Service

- Revert any changes you have made on `docker-compose.yaml` in [Prepare the Database](#prepare-the-database). (Changes made in [Configuration](#configuration) should be kept.)
- Run `docker-compose up`.

## Backup

- Make sure the service is started.
- Create a dump of the datebase: `docker exec -it $(docker ps -aqf'name=slackarchive_postgres') /backup.sh`.

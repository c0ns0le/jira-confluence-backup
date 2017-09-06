FROM alpine

MAINTAINER stefan.lapers@intersoft.solutions

COPY bin/jira-confluence-backup_linux_amd64 /jira-confluence-backup

RUN apk --update --no-cache add ca-certificates

CMD ["jira-confluence-backup"]
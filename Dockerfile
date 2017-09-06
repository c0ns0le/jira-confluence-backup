FROM alpine

MAINTAINER stefan.lapers@intersoft.solutions

COPY bin/jira-confluence-backup_linux_amd64 /jira-confluence-backup

CMD ["jira-confluence-backup"]
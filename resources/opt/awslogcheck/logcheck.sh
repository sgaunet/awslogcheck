#!/bin/bash


echo "Launch awslogcheck" 
echo "------------------"

/opt/awslogcheck/awslogcheck -t 3600 -g $LOGGROUP  -c /opt/awslogcheck/cfg.yaml > /tmp/result.md
rc=$?

if [ "$rc" != "0" ]
then  
  echo "Error occurend when executing awslogcheck"
fi

mdtohtml /tmp/result.md /tmp/result.html
rc=$?

if [ "$rc" != "0" ]
then  
  echo "Error occurend when executing mdtohtml"
  exit 1
fi

cat /tmp/result.html | curl -s --user "api:${MAILGUN_APIKEY}" https://api.mailgun.net/v3/${MAILGUN_DOMAIN}/messages -F from="${MAILGROM}" -F to="${MAILTO}" -F subject=${SUBJECT} -F html="<-" "$@"

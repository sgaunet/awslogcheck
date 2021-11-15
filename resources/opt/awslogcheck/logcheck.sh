#!/bin/bash

source /opt/awslogcheck/check-smtp-conf.sh # Will export CFG_MAILGUN and CFG_SMTP

echo "Launch awslogcheck" 
echo "------------------"

/opt/awslogcheck/awslogcheck -t 3600 -g $LOGGROUP  -c /opt/awslogcheck/cfg.yaml > /tmp/result.md
rc=$?

if [ "$rc" != "0" -a "$rc" != "200" ]
then  
  echo "Error occurend when executing awslogcheck" | tee /tmp/result.md
fi


if [ "$rc" = "200" ]
then  
  echo "Every logs have been filtered"
  exit 0
fi

sed -i ':a;N;$!ba;s/\n/<br>\n/g'  /tmp/result.md
mdtohtml /tmp/result.md /tmp/result.html
rc=$?

if [ "$rc" != "0" ]
then  
  echo "Error occurend when executing mdtohtml" | tee /tmp/result.html
fi

if [ "$CFG_MAILGUN" = "0" ]
then
  cat /tmp/result.html | curl -s --user "api:${MAILGUN_APIKEY}" https://api.mailgun.net/v3/${MAILGUN_DOMAIN}/messages -F from="${FROM_EMAIL}" -F to="${MAILTO}" -F subject=${SUBJECT} -F html="<-" "$@"
fi

if [ "$CFG_SMTP" = "0" ]
then
  mutt -e "set content_type=text/html" -s "$SUBJECT" -- ${MAILTO} < /tmp/result.html
fi

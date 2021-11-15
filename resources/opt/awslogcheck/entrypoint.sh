#!/bin/bash

function checkVarIsNotEmpty
{
  var="$1"
  eval "value=\$$var"
  if [ -z "$value" ]
  then  
    echo "$var not set. EXIT 1"
    exit 1
  fi
}

# Check vars
checkVarIsNotEmpty MAILTO
checkVarIsNotEmpty SUBJECT
checkVarIsNotEmpty LOGGROUP

source /opt/awslogcheck/check-smtp-conf.sh

if [ "$CFG_SMTP" = "0" ]
then
  # COnfigure mutt
  cat > $HOME/.muttrc << EOF
  set ssl_starttls = yes 
  set smtp_url="smtp://${SMTP_LOGIN}@${SMTP_SERVER}:${SMTP_PORT}/"
  set smtp_pass="${SMTP_PASSWORD}"

  #set smtp_authenticators = "login"

  set from="${FROM_EMAIL}"
  set use_from="yes"
  set realname="${REALNAME}"
  set send_charset="utf-8"
EOF
fi

exec $@
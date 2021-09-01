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
checkVarIsNotEmpty MAILGUN_APIKEY
checkVarIsNotEmpty MAILGUN_DOMAIN
checkVarIsNotEmpty MAILGROM
checkVarIsNotEmpty MAILTO
checkVarIsNotEmpty SUBJECT
checkVarIsNotEmpty LOGGROUP

exec $@
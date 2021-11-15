#!/usr/bin/env bash

if [ -z "${MAILGUN_APIKEY}" -o -z "${MAILGUN_DOMAIN}" ]
then
  CFG_MAILGUN="1"
else
  CFG_MAILGUN="0"
fi

if [ -z "${SMTP_LOGIN}" -o -z "${SMTP_SERVER}" -o -z "${SMTP_PASSWORD}" ]
then
  CFG_SMTP="1"
else
  if [ ! -z "${SMTP_LOGIN}" -a ! -z "${SMTP_SERVER}" -a ! -z "${SMTP_PASSWORD}" ]
  then
    CFG_SMTP="0"
  else
    CFG_SMTP="1"
  fi
fi

if [ "$CFG_MAILGUN" = "1" -a "$CFG_SMTP" = "1" ]
then
  echo "ERROR: Choose a method to send the report by mail..."
  exit 1
fi
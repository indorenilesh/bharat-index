FROM postgres:16-alpine

COPY schema.sql /docker-entrypoint-initdb.d/01-schema.sql
COPY entrypoint.sh /usr/local/bin/db-entrypoint.sh

RUN chmod +x /usr/local/bin/db-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/db-entrypoint.sh"]

CMD ["postgres"]

EXPOSE 5432

#!/bin/sh

for file in *puml; do
    echo Processing ${file}...;
    java -jar plantuml.jar -tsvg  ${file};
done

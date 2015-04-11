
var myApp = angular.module("builder", ["dndLists"])
myApp.controller("tasksbuilder", function($scope) {
    $scope.models = {
        selected: null,
        templates: [
            {type: "item", name: "Task", id: 1},
            {type: "container", name: "Parallel", id: 2, columns: [[], []]}
        ],
        dropzones: {
            "commands": [
                {
                    "type": "item",
                    "id": 1,
                    "cmd": "go fmt .",
                },
                {
                    "type": "item",
                    "id": 1,
                    "cmd": "goimports -w .",
                },
                {
                    "type": "container",
                    "id": 1,
                    "columns": [
                        [
                            {
                                "type": "item",
                                "id": "1",
                                "cmd": "go lint .",
                            },
                            {
                                "type": "item",
                                "id": "2",
                                "cmd": "go vet .",
                            }
                        ],
                        [
                            {
                                "type": "item",
                                "id": "3",
                                "cmd": "go errcheck .",
                            }
                        ]
                    ]
                },
                {
                    "type": "item",
                    "id": "6",
                    "cmd": "go otherchecks .",
                }
            ]
        }
    };

    $scope.$watch('models.dropzones', function(model) {
        $scope.modelAsJson = angular.toJson(model, true);
    }, true);

});

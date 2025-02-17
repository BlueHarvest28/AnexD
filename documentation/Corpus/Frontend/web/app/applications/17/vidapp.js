'use strict';
var ready = false;
var YT = null;
var player;

function onYouTubeIframeAPIReady() {
  player = new YT.Player('player', {
    videoId: 'M7lc1UVf-VE',
    width: 582,
    height: 383,
    playerVars: {
      color: 'white',
      autoplay: 0,
      controls: 1
    },
    events: {
      'onReady': onPlayerReady,
    }
  });
}

function onPlayerReady(event) {
  event.target.playVideo();
  console.log("Play Video - onReady()");
}

function curTime() {
  var time = Math.floor(player.getCurrentTime());
  alert(time);
  console.log("Current Time - curTime()");
  //return curTime;
}

function getDuration() {
  player.getDuration();
  console.log("Duration of vid - getDuration()");
}

// function writeComment(){
//   var userInput = document.getElementById('userInput').value;
//   document.getElementById('dispComment').innerHTML = userInput;
// userInput.onKeyUp = function(){
//   document.getElementById('dispTime').innerHTML = inputBox.curTime();
// }
// }
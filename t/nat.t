#!/usr/bin/perl

use strict;
use Test::More;
use Test::Differences;
use lib 't';
use Test_Netspoc;

my ($title, $in, $out1, $head1, $out2, $head2, $out3, $head3);

############################################################
$title = 'Multiple dynamic NAT at ASA';
############################################################

# Soll nur einen nat-Index pro Interface verwenden.

$in = <<END;
network:Test =  { ip = 10.9.1.0/24; nat:C = { ip = 1.1.1.1/32; dynamic;} }

router:filter = {
 managed;
 model = ASA;
 interface:Test = {
  ip = 10.9.1.1;
  hardware = inside;
 }
 interface:X = { ip = 10.9.3.1; hardware = outside; bind_nat = C;}
 interface:Y = { ip = 10.9.2.1; hardware = DMZ50; bind_nat = C;}
}

network:X = { ip = 10.9.3.0/24; }
network:Y = { ip = 10.9.2.0/24; }

service:IP = ip;

policy:test = {
 user = network:X, network:Y;
 permit src = user; 
	dst = network:Test;
	srv = service:IP;
}

END

$out1 = <<END;
global (outside) 1 1.1.1.1 netmask 255.255.255.255
nat (inside) 1 10.9.1.0 255.255.255.0
global (DMZ50) 1 1.1.1.1 netmask 255.255.255.255
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = 'Dynamic NAT for network with static nat for hosts at ASA';
############################################################

$in = <<END;
network:Test =  {
 ip = 10.9.1.0/24; 
 nat:C = { ip = 1.1.1.16/28; dynamic;}
 host:H = { ip = 10.9.1.33; nat:C = { ip = 1.1.1.23; } }
}

router:filter = {
 managed;
 model = ASA;
 interface:Test = {
  ip = 10.9.1.1;
  hardware = inside;
 }
 interface:X = { ip = 10.9.3.1; hardware = outside; bind_nat = C;}
}

network:X = { ip = 10.9.3.0/24; }

service:IP = ip;
service:HTTP = tcp 80;

policy:test = {
 user = network:X;
 permit src = user;   dst = host:H;       srv = service:IP;
 permit src = host:H; dst = user;         srv = service:HTTP;
 permit src = user;   dst = network:Test; srv = service:HTTP;
}
END

$out1 = <<END;
access-list inside_in extended permit tcp host 10.9.1.33 10.9.3.0 255.255.255.0 eq 80
access-list inside_in extended deny ip any any
access-group inside_in in interface inside
END

$out2 = <<END;
access-list outside_in extended permit ip 10.9.3.0 255.255.255.0 host 1.1.1.23
access-list outside_in extended permit tcp 10.9.3.0 255.255.255.0 1.1.1.16 255.255.255.240 eq 80
access-list outside_in extended deny ip any any
access-group outside_in in interface outside
END

$out3 = <<END;
global (outside) 1 1.1.1.16-1.1.1.31 netmask 255.255.255.240
nat (inside) 1 10.9.1.0 255.255.255.0
static (inside,outside) 1.1.1.23 10.9.1.33 netmask 255.255.255.255
END

$head1 = (split /\n/, $out1)[0];
$head2 = (split /\n/, $out2)[0];
$head3 = (split /\n/, $out3)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);
eq_or_diff(get_block(compile($in), $head2), $out2, $title);
eq_or_diff(get_block(compile($in), $head3), $out3, $title);

############################################################
done_testing;
